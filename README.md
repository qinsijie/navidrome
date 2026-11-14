<a href="https://www.navidrome.org"><img src="resources/logo-192x192.png" alt="Navidrome logo" title="navidrome" align="right" height="60px" /></a>

# Navidrome Music Server（飞牛 fnOS · Hi-Fi 源码输出 · 聚合音源定制版）

本仓库基于开源项目 [Navidrome](https://www.navidrome.org)（上游基准提交 `ec2e0a9e`，当前套件版本 `v0.64.33`）进行深度定制开发，面向**飞牛 fnOS 私有云生态**、**发烧友级物理声卡直推**与**移动端 WebView 稳定性**场景增强，并保留对标准 Subsonic / OpenSubsonic 客户端的完整兼容。

> **架构与开发红线**：所有二次开发与重构需严格遵循仓库根目录下的 [AGENTS.md](AGENTS.md) 规范。关于全仓代码健康度评估与已知问题清单，请参阅 [CODE_REVIEW_REPORT.md](CODE_REVIEW_REPORT.md)。

---

## 核心定制特性

### 1. 飞牛 fnOS 单默认主账号免登录架构
- **开箱零登录直达**：当未显式关闭 `ND_AUTO_LOGIN` 时，服务端渲染入口页自动注入默认主管理员凭据（[`handleDefaultAdminLogin`](server/auth.go) -> `window.__APP_CONFIG__.auth`），从飞牛桌面图标、FN Connect 或局域网浏览器访问即可直接进入播放界面。
- **保留多实体外键关联**：底层数据库依然完整保留 `user_id` 关联，个人歌单、红心收藏（Starring）、播放历史以及在线曲目 ShadowTrack 均可正常读写。
- **飞牛快捷鉴权接口**：支持 `/auth/fnos-login`（代理身份头直登 / 主账号回落）及 `/auth/fnos-reset-password`，内置简体中文（`ND_DEFAULTLANGUAGE=zh-Hans`）默认语言。
  > **安全提示**：默认配置面向信任局域网与飞牛内嵌免登环境；如需直接暴露至不可信公网，请务必配置 `ND_AUTO_LOGIN=false` 并前置反向代理认证（同时核查 [CODE_REVIEW_REPORT.md](CODE_REVIEW_REPORT.md) 中的高危安全项）。

### 2. 双轨独立播控与多终端物理隔离（Local vs Remote）
系统将“本机浏览器/App 播放”与“客厅 Hi-Fi 物理声卡播放”拆分为两条完全独立互不干扰的轨道：

| 维度 | Local（本机播放） | Remote（HQPlayer / DLNA 远端声卡） |
| :--- | :--- | :--- |
| **持久化维度** | 按终端 `client_id`（`localStorage.nd_client_unique_id`）+ 自定义备注名隔离持久化至 `terminal_queues.json` | 服务端 [`PlayerSession`](core/output/session.go) 与 [`UserSession`](core/output/user_session.go) 维护共享硬件队列 |
| **多端协同与隔离** | 多台手机/PC 刷新恢复各自队列与断点，**严禁跨端广播**防止触发移动端自动播放异常 | 单主账号硬件协同，废除多用户 409 抢占弹窗，通过 SSE（`userPlaybackState`）+ 1s 兜底轮询多端同步遥控视图 |
| **切入与接力策略** | 切换回本机时自动恢复本地歌单快照或向服务端拉取该终端列表 | **只读拉取远端设备现有队列，严禁自动覆写**；仅在播放列表面板显式点击「导入本机列表」时推送 |
| **本地 DOM 行为** | 全局唯一常驻 `<audio>` + Web Audio 微音频保活通道 | 本地 `<audio>` 强制静音、暂停并置空音频流（拦截为极小静音 Data URI），零本地拉流，纯遥控器模型 |

### 3. HQPlayer & DLNA 发烧友级源码推流管线
- **HQPlayer XML-over-TCP 直控（默认端口 `4321`）**：
  - 完整实现 `<State/>` 探活、`<Stop/>`、`<PlaylistClear/>`、`<PlaylistAdd/>`、`<Play/>`、`<Seek/>`、`<Volume/>`（`-60dB ~ 0dB` 映射）指令序列，切歌及停止内置 **500ms ALSA/Diretta DMA 排空等待**与 `Empty transport` 挂载重试。
  - **Gapless 无缝续播（`v0.64.32+`）**：在当前曲目结束前约 15 秒自动向 HQPlayer 预入队下一首（`<PlaylistAdd>`），并通过 `<State/>` 轮询曲目索引跃迁实现硬件缓冲无间断接力。
  - **双传输模式自适应**：
    - **类 Roon 恒定码率 PCM/WAV 模式（`streamMode: "pcm"`，默认）**：通过 [`ServePCMStream`](core/output/pcm_stream.go) 按原曲原生采样率/位深即时解码生成标准 44B PCM / 68B `WAVE_FORMAT_EXTENSIBLE` WAV 流，支持 `HEAD` 探测、`HTTP Range` 与 **1.12x 恒定令牌桶限速推流**；遇到 `.dsf` / `.dff` 原生 DSD 时自动旁路保持源码直通。
    - **原始文件源码直通模式（`streamMode: "raw"`）**：以真实音频文件格式 + **1.30x 比特率限速 & 1MB 首冲缓冲**（[`PacedResponseWriter`](core/stream/pacer.go)）直喂物理渲染器；同时本地 `/rest/stream` Seekable 流配备 `burstResponseWriter`（**3MB 首冲预热**）。
- **DLNA / UPnP MediaRenderer**：
  - 支持 SSDP（`239.255.255.250:1900`）局域网自动发现与手动填写 Description URL 添加渲染器；基于 SOAP `AVTransport:1` 与 `RenderingControl:1` 控制。
  - 自动携带 `?format=<ext>` 精确推断 `DIDL-Lite` `protocolInfo` MIME（涵盖 `flac`/`wav`/`dsf`/`dff`/`ape`/`wv`/`mp3`/`m4a`/`ogg`），支持 Pause 失败自动回落到 Stop，并自动探测可达局域网出口 IP（[`DetectOutboundIP`](core/output/manager.go)）。
- **全链路可视化诊断**：暴露 `/api/output/debug` 内存环形事件日志（记录 `pcm_head` / `pcm_start` / `pcm_end` / `pcm_error` / `hqp_*` / `dlna_*` 等打点），并提供含 `stream_debug` 编译标签的 Debug 版 `.fpk`。

### 4. MusicEngine 第三方聚合音源引擎与轻量级边听边存
- **Goja JS 沙箱与原生聚合检索（[`core/musicengine`](core/musicengine)）**：
  - 内置 Goja Worker 池与 Polyfill 沙箱（支持落雪 `lx` / `_lx_events` / `userApi` 规范，注入 AES/RSA/MD5/UTF-8/`__nativeHttpSync`/`fetch` 等能力），支持本地脚本上传、URL 订阅导入、在线更新检查、一键分享导出与测活。
  - 内置原生平台聚合解析与发现接口（支持 `kw` 酷我、`kg` 酷狗、`tx` QQ 音乐、`wy` 网易云的搜索、搜索提示、精选歌单与热门榜单）。
- **ShadowTrack 统一入库与流式代理**：
  - 使用虚拟协议路径 `online://<source>/<songId>/<quality>`（ID 格式 `online:<source>:<songId>`）入库为统一 `MediaFile`（[`shadow.go`](core/musicengine/shadow.go)），并在仓库缺失清理逻辑中豁免保护（[`mediafile_repository.go`](persistence/mediafile_repository.go)），从而无缝复用本地歌单、红心、最近播放以及 HQPlayer/DLNA 推流。
  - [`StreamRelay`](core/musicengine/relay.go) + [`SlabRingBuffer`](core/musicengine/buffer.go)（64KB Slab 滑窗）承接 HTTP Range 与 Seek 重锚，边播边异步写入 `<CacheFolder>/online_music`（默认上限 `1 GB`，范围 `256 MB ~ 20 GB`，超限按访问时间 LRU 自动淘汰，支持缓存命中直接磁盘 HTTP Range 秒开）。

### 5. 移动端 / WebView 稳定性与系统级集成（AudioGuard）
- **空源全局拦截与异常收敛**：在 [`ui/index.html`](ui/index.html) 底层劫持 `HTMLMediaElement.prototype.src` 与 `setAttribute('src', ...)`，彻底阻断空字符串、`/` 或 `/app/` 相对根路径导致的解码器报错；静默收敛切流引发的 `AbortError`。
- **熄屏微音频保活 & 系统媒体中心联动**：[`Player.jsx`](ui/src/audioplayer/Player.jsx) 维持单实例 `<audio>`、Web Audio 极低增益（`0.00001`）振荡保活通道、WakeLock 屏幕唤醒锁，完整对接 `navigator.mediaSession` 并在锁屏歌手栏实时投影同步歌词（800ms AVRCP 防刷屏节流）。
- **智能容错恢复**：非用户触发的被动 `pause` 延迟 `1500ms` 自动续播，捕获 `NotAllowedError` 立即执行单次熔断锁定；内置停滞看门狗（`_t` 流重锚）与连续失败 3 首雪崩保护；针对触屏进度条拖拽修复了 `rc-slider` 丢失 `touchend` 问题（[`ui/patches/rc-slider+9.7.5.patch`](ui/patches/rc-slider+9.7.5.patch)）。
- **轻量控制台（vConsole）**：URL 含 `debug`、`localStorage.nd_debug=1` 或三指长触页面即可呼出可拖拽深色 `vConsole`。
- **飞牛套件 FFmpeg 自适应兜底**：[`core/ffmpeg/ffmpeg.go`](core/ffmpeg/ffmpeg.go) 自动回退检索 `/usr/trim/bin/ffmpeg`、`/usr/bin/ffmpeg`、`/var/apps/*/target/bin/ffmpeg` 等路径。

---

## 目录结构概览（自定义增强模块）

```text
navidrome/
├── .fnos/                              # 飞牛 fnOS 应用中心打包清单、启停脚本与存储卷池化配置
│   ├── cmd/main                        # 启动脚本：数据目录准备、/vol* 媒体池软链接、环境注入
│   ├── cmd/uninstall_callback          # 卸载清理（仅清理数据库/缓存/软链，严禁触碰真实媒体文件）
│   ├── config/navidrome.toml           # 飞牛默认运行配置（0.0.0.0:4533, zh-Hans, 并发转码限制）
│   ├── manifest / manifest.json        # 应用元信息（版本 0.64.33，桌面入口）
│   └── ui/ / wizard/                   # 飞牛桌面图标与卸载向导
├── .github/workflows/build-fpk.yml     # CI 自动构建 Release 与 Debug (.fpk) 制品流水线
├── core/
│   ├── ffmpeg/ffmpeg.go                # 飞牛套件及系统常见路径 FFmpeg 自动探测兜底
│   ├── musicengine/                    # 第三方聚合音源 Goja 沙箱、平台搜索、ShadowTrack、Relay 与 LRU 缓存
│   ├── output/                         # HQPlayer/DLNA 控制器、PlayerSession、DualSession、PCM/WAV 推流、终端队列存储
│   ├── scrobbler/play_tracker.go       # 播放状态追踪扩展（暴露 GetActivePlayTracker 等）
│   └── stream/                         # 令牌桶匀速限速 PacedResponseWriter、3MB BurstResponseWriter、stream_debug 标签
├── server/
│   ├── auth.go / serve_index.go        # 单主账号免登录注入（ND_AUTO_LOGIN）及 /auth/fnos-* 接口
│   ├── events/                         # UserPlaybackState SSE 广播事件与非阻塞 Broker 队列扩容
│   ├── nativeapi/                      # /api/output/*、/api/player/terminal-queue、/api/system/paths 路由
│   └── subsonic/                       # online:// 协议支持、转码回落及 XML &amp; 实体参数兼容
└── ui/
    ├── index.html                      # HTMLMediaElement 空源拦截、vConsole、异常收敛前置脚本
    ├── patches/rc-slider+9.7.5.patch   # 移动端触屏进度条防卡死补丁
    └── src/
        ├── audioplayer/                # Player 增强、OutputDeviceSelector 分区选择器、AudioTitle 等
        ├── onlinemusic/                # 聚合搜索、精选歌单、热门榜单、音源配置与缓存管理界面
        ├── personal/AudioSettings.jsx  # HQPlayer（PCM/Raw）配置测试与 DLNA 扫描管理面板
        └── recentlyplayed/             # 本地最近播放记录视图
```

---

## 构建与部署指南

### 1. 飞牛 fnOS 一键安装包（CI 自动产物）
推送至 `master`/`main` 或 `v*` 标签后，[`.github/workflows/build-fpk.yml`](.github/workflows/build-fpk.yml) 将自动产出一组双版本 `.fpk` 安装包（Workflow Artifact `navidrome-fnos-fpk`）：
- `navidrome-x86_64-v<VER>.fpk`：**Release 生产版**（`-tags=netgo,sqlite_fts5`）
- `navidrome-x86_64-v<VER>-debug.fpk`：**Debug 诊断版**（`-tags=netgo,sqlite_fts5,stream_debug`，包含 `Audio Stream Initial Burst Primed` 等详细流媒体诊断日志）

安装后启动脚本 [`.fnos/cmd/main`](.fnos/cmd/main) 会自动扫描 `/vol*` 下的 `Music`、`music`、`Media/Music`、`media/music`、`我的音乐`、`音乐` 目录，并将其以只读/隔离软链接池形式挂载至 `${TRIM_PKGVAR}/music_pool`。

### 2. 本地从源码编译
构建环境依赖：**Go 1.27+**（见 CI workflow 基准）、**Node.js 20+**、**`libtag1-dev`**（TagLib C++ 开发库）、`gcc`/`g++` 以及运行时 **`ffmpeg`**。

```bash
# 1. 构建前端静态资源（含自动执行 rc-slider 触屏补丁）
cd ui
npm ci
npm run build
cd ..

# 2. 编译后端二进制（嵌入式挂载构建好的前端静态资源）
go build -tags=netgo,sqlite_fts5 -ldflags="-s -w" -o navidrome .

# 可选：启用流媒体调试日志标签
go build -tags=netgo,sqlite_fts5,stream_debug -ldflags="-s -w" -o navidrome-debug .

# 3. 运行（参考飞牛默认配置）
./navidrome --configfile .fnos/config/navidrome.toml
```

---

## 运行配置与环境变量

除上游 Navidrome 原生参数（`ND_*` / `navidrome.toml`）外，本定制版涉及以下关键配置与环境变量：

| 变量 / 配置项 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `ND_AUTO_LOGIN` | 开启（非 `false`/`0`/`no`） | 是否默认向 Web 首页注入首个管理员凭据实现零登录直达；公网开放访问时建议设为 `false` |
| `ND_DEFAULTLANGUAGE` | `zh-Hans` | 默认界面语言（本分支已将 Go 默认值改为 `zh-Hans`） |
| `ND_MUSICFOLDER` | `./var/music_pool` | 曲库根目录；飞牛启动脚本会自动注入 `${TRIM_PKGVAR}/music_pool` |
| `ND_DATAFOLDER` | `./var/data` | 存储 SQLite 数据库、`audio_output.json`（输出设备配置）、`terminal_queues.json`（终端本地队列） |
| `ND_CACHEFOLDER` | `./var/data/cache` | 存储上游转码缓存、音源脚本（`musicengine/scripts`）及在线试听 LRU 缓存（`online_music`） |
| `TRIM_PKGVAR` | `/vol1/@appdata/navidrome`（脚本回落） | 飞牛 fnOS 套件持久化数据根目录环境变量 |

---

## 自定义扩展 API 速览

### 认证管理
- `POST` / `GET /auth/fnos-login`：飞牛快捷免登录凭证签发（支持代理头或回落至首个管理员）
- `POST /auth/fnos-reset-password`：快捷密码重置（注意仅限可信内网使用）

### 音频输出与终端同步
- `GET /api/output/devices` / `POST /api/output/device` / `POST /api/output/device/update`：列出、切换及编辑输出区域（`local` / `hqplayer` / DLNA UDN）
- `POST /api/output/hqplayer/config` / `POST /api/output/hqplayer/test`：HQPlayer 主机/端口/`streamMode`（`pcm` / `raw`）配置与 TCP 4321 XML 连通性测试
- `POST /api/output/dlna/scan` / `POST /api/output/dlna/add` / `DELETE /api/output/dlna/{id}`：SSDP 扫描与手动 DLNA 渲染器增删
- `GET` / `POST /api/output/queue`、`POST /api/output/play`、`POST /api/output/control`（及 `/pause`、`/seek`、`/volume`）：双通道队列读写与远端硬件控制
- `GET /api/output/status` / `GET /api/output/debug`：查询远端实时状态及最近 40 条输出/PCM 调试打点
- `GET` / `HEAD /api/output/stream/{id}`（含 `.wav` 后缀与 `?mode=pcm` / `?format=` 参数）：物理声卡直推免 JWT 音频流端点
- `GET` / `POST /api/player/terminal-queue`：按终端 `clientId` 隔离持久化的 Local 播放队列读写
- `GET /api/system/paths`：管理员浏览飞牛存储卷与音乐池路径

### 第三方聚合音源（`/api/musicengine/*`）
- 音源脚本管理：`GET /sources`、`POST /sources/upload`、`POST /sources/import-url`、`POST /sources/{id}/activate`、`DELETE /sources/{id}`、`GET /sources/{id}/download`、`POST /sources/{id}/check-update`、`POST /sources/ping`
- 在线音乐发现与入库：`GET /search`、`GET /search/tips`、`GET /songlists`、`GET /songlist`、`GET /playlists/detail`、`GET /leaderboards`、`GET /leaderboard`、`POST /shadow`
- 流代理与缓存：`GET /stream` / `/stream/{id}`、`GET /cache`、`POST /cache/config`、`DELETE /cache`

---

## 测试与质量状态说明

- 前端工程 (`ui/`) 提供 Vitest 单元测试与构建检查（`cd ui && npm test`、`npm run lint`、`npm run type-check`）；当前仓库中部分定制交互尚未同步更新旧版单测（详见 [CODE_REVIEW_REPORT.md](CODE_REVIEW_REPORT.md) 的实际测试运行结果记录）。
- 后端自定义单元测试位于 [`core/musicengine/musicengine_test.go`](core/musicengine/musicengine_test.go)、[`core/musicengine/search_test.go`](core/musicengine/search_test.go) 与 [`core/output/hqplayer_test.go`](core/output/hqplayer_test.go)，可使用 `go test ./core/musicengine/... ./core/output/...` 执行。

---

## 上游 Navidrome 原始信息与参考链接

本项目核心曲库扫描、元数据刮削、多客户端 Subsonic API 与基础 Web UI 源自优秀的开源项目 **[Navidrome](https://www.navidrome.org)**：

- 官方网站与文档：https://www.navidrome.org/docs/
- Subsonic API 兼容性说明：https://www.navidrome.org/docs/developers/subsonic-api/
- 上游 GitHub 仓库：https://github.com/navidrome/navidrome
- 社区交流：[Discord](https://discord.gg/xh7j7yF) · [Reddit (/r/navidrome)](https://www.reddit.com/r/navidrome/)
