# Navidrome 定制版代码全面审查报告（Code Review Report）

- **审查基准提交**：`6898ccf7`（`v0.64.33`），相对上游基准提交 `ec2e0a9e` 变更范围：`122 files changed, 19508 insertions(+), 1927 deletions(-)`。
- **审查范围**：全量覆盖飞牛 fnOS 打包与鉴权定制（`.fnos/*`、`server/auth.go`、`server/serve_index.go`）、音频输出与 Roon-like PCM/源码推流模块（`core/output/*`、`core/stream/*`、`server/nativeapi/output.go`、`server/subsonic/*`）、第三方聚合音源引擎（`core/musicengine/*`）、事件总线与播放追踪（`server/events/*`、`core/scrobbler/*`、`persistence/mediafile_repository.go`）、以及前端播放器与扩展视图（`ui/index.html`、`ui/src/audioplayer/*`、`ui/src/onlinemusic/*`、`ui/src/reducers/playerReducer.js`、`ui/src/eventStream.js` 等）。

---

## 一、实测构建与测试执行现状

1. **Go 后端编译与单测（未执行）**：
   - 当前工作区容器未安装 `go` 编译器（`which go` 及 `/usr/local/go/bin` 均未检出），因此 `go test`（含 [`core/musicengine/musicengine_test.go`](core/musicengine/musicengine_test.go)、[`core/musicengine/search_test.go`](core/musicengine/search_test.go)、[`core/output/hqplayer_test.go`](core/output/hqplayer_test.go) 及 `-race` 竞态检测）在本环境无法执行，需在含 Go 1.27 + `libtag1-dev` 的 CI 或开发机运行验证。
2. **前端 Vitest 单测（部分失败）**：
   - 运行改动的 3 个核心关联测试文件 `./node_modules/.bin/vitest run --no-color src/audioplayer/PlayerToolbar.test.jsx src/layout/AppBar.test.jsx src/transcode/decisionService.test.js`：
     - [`src/layout/AppBar.test.jsx`](ui/src/layout/AppBar.test.jsx)：6/6 通过；
     - [`src/transcode/decisionService.test.js`](ui/src/transcode/decisionService.test.js)：25/25 通过；
     - [`src/audioplayer/PlayerToolbar.test.jsx`](ui/src/audioplayer/PlayerToolbar.test.jsx)：**4 失败 / 5 通过**（移动端用例失败）。
   - 单独运行 `./node_modules/.bin/vitest run --no-color src/eventStream.test.js`：**1/1 失败**。
   - 执行全量 `./node_modules/.bin/vitest run --no-color`（耗时 107.09s，共 93 个测试文件 / 852 条用例）：**`Test Files 7 failed | 86 passed (93)`**、**`Tests 30 failed | 822 passed (852)`**。除上述 2 个文件外，其余 5 个失败文件为 `useScrollRestoration.test.jsx`（3 例 hook 超时）、`jsonValidation.test.js`（1 例失败）、`SharePlayer.test.jsx`（5 例失败）、`NowPlayingPanel.test.jsx`（10 例失败）和 `useScanElapsedTime.test.jsx`（6 例 `afterEach` hook 超时）。

---

## 二、问题与优化汇总索引

| 严重级别 | 数量 | 主要风险领域 |
| :--- | :---: | :--- |
| **P0 严重（Critical）** | 3 | 未鉴权路由越权（MusicEngine 全栈操作 + ShadowTrack 任意本地文件读取）、无鉴权/无限流 `/auth/fnos-*` 账号接管 |
| **P1 高危（High）** | 12 | 公开分享页泄露管理员凭据、Relay Seek 数据竞态与老 Buffer 泄漏、32MB 滑窗高码率断流、Goja VM 跨协程并发 Panic、残缺流写入永久缓存、PCM HTTP 协议帧破坏、SSE 无写超时挂起、远端双状态机分裂、`clear` 失效、虚拟时钟双倍计时等 |
| **P2 中危（Medium）** | 5 | 音源分源断裂（`mg`/`qs`）、Scrobbler 多端互相覆盖、相对路径锚定不统一、`.fnos` 打包脚本错配、前端单元测试回归失败 |
| **P3 优化与卫生（Optimization）** | 5 | 每次请求分配 3MB 缓冲切片、并发写 JSON 无锁、缺失 `Unwrap()`、死代码清理、`Player.jsx` 巨型组件拆分与全局异常吞没 |

---

## 三、P0 严重问题（必须优先修复）

### [P0-1] `/api/musicengine/*` 全路由未挂载鉴权，叠加 Goja 注入与 SSRF 导致未授权写库与内网探测
- **位置**：
  - [`cmd/root.go#L132`](cmd/root.go#L132)
  - [`core/musicengine/router.go#L35-L71`](core/musicengine/router.go#L35-L71)
  - [`core/musicengine/engine.go#L223-L258`](core/musicengine/engine.go#L223-L258), [`#L347-L357`](core/musicengine/engine.go#L347-L357), [`#L554-L605`](core/musicengine/engine.go#L554-L605)
  - [`core/musicengine/sandbox.go#L196-L266`](core/musicengine/sandbox.go#L196-L266), [`#L356-L420`](core/musicengine/sandbox.go#L356-L420)
  - [`core/output/dlna.go#L110-L128`](core/output/dlna.go#L110-L128)
- **原因与影响**：
  1. `cmd/root.go:132` 直接将 `musicengine.NewRouter` 挂载于根路由器 `a.MountRouter(..., consts.URLPathNativeAPI+"/musicengine", ...)`，完全绕过 [`server/nativeapi/native_api.go#L77-L94`](server/nativeapi/native_api.go#L77-L94) 的 `Authenticator`、`JWTRefresh` 与 `adminOnlyMiddleware` 保护（即使系统设置 `ND_AUTO_LOGIN=false` 也完全裸奔）。
  2. `core/musicengine/router.go` 中的敏感管理操作（`POST /sources/upload`、`POST /sources/import-url`、`POST /sources/{id}/activate`、`DELETE /sources/{id}`、`POST /cache/config`、`DELETE /cache`、`POST /shadow`）均无认证与管理员鉴权。
  3. `engine.go:223-258` 与 `347-357` 使用 `fmt.Sprintf` 将未经转义的 `source`、`quality`、`songID`、`query` 拼接进单引号 JS 字符串中并交由 Goja 执行，构成 **JS 代码注入**。注入代码可调用 `sandbox.go` 提供且未限制私网/回环地址/Protocol 的 `__nativeHttpSync` 与 `fetch`；同时 `/sources/import-url` 与 `POST /api/output/dlna/add` (`dlna.go:110`) 亦可发起任意 HTTP 请求，存在 **SSRF** 与持久化恶意音源风险。
- **修复建议**：
  1. 将除公开流媒体回源外的所有 `musicengine` 管理及发现接口迁入 `nativeapi` 受保护路由组，脚本上传/删除/导入/缓存配置限定 `adminOnlyMiddleware`。
  2. Goja JS 调用改用 Go 函数参数绑定（`goja.AssertFunction` + `fn(goja.Undefined(), vm.ToValue(arg))`），彻底消除 `fmt.Sprintf` 拼接脚本。
  3. 对 `/sources/import-url`、`/output/dlna/add`、沙箱 `fetch`/`__nativeHttpSync` 的目标地址增加 Scheme 白名单（仅 `http`/`https`）及内网/回环 CIDR 防护（按需为本地 DLNA 单独放行合法局域网或提示确认）。

### [P0-2] ShadowTrack 任意路径写入与免鉴权 `/api/output/stream/{id}` 联动可致任意本地文件读取
- **位置**：
  - [`core/musicengine/shadow.go#L79-L87`](core/musicengine/shadow.go#L79-L87), [`#L136-L141`](core/musicengine/shadow.go#L136-L141)
  - [`model/mediafile.go#L255-L257`](model/mediafile.go#L255-L257)
  - [`server/nativeapi/output.go#L118-L179`](server/nativeapi/output.go#L118-L179), [`#L247-L248`](server/nativeapi/output.go#L247-L248)
- **原因与影响**：
  1. `POST /api/musicengine/shadow`（匿名可达）调用 `BuildDefensiveShadowTrack`，其中 `L79-87` 直接采信请求 JSON 中的 `UniversalID` 和 `ProtocolPath` 而不强制校验 `"online://"` 前缀，随后 `EnsureShadowMediaFile` 直接将其写入 `media_file` 数据库。
  2. 公开免鉴权接口 `GET/HEAD /api/output/stream/{id}`（为方便 HQPlayer/DLNA 拉流设计，未要求 JWT/Subsonic Salt）在 `mf.Path` 不以 `"online://"` 开头时，直接调用 `mf.AbsolutePath()`（即 `filepath.Join(mf.LibraryPath, mf.Path)`，未阻断 `../` 路径穿越）并执行 `os.Open(filePath)` + `http.ServeContent` 返回文件内容。两者串联形成零认证任意文件读取链。
- **修复建议**：
  1. `BuildDefensiveShadowTrack` 与 `EnsureShadowMediaFile` 中强制校验并重写 `ID = BuildUniversalID(source, songID)` 及 `Path = BuildProtocolPath(source, songID, quality)`，拒绝任何非法 `source`/`songID`（仅允许白名单字母数字格式），严禁采信客户端传入的任意 `ProtocolPath`。
  2. 在 `/api/output/stream/{id}` 中校验证书、签名或一次性短效推流 Token，并在本地文件打开前校验 `filepath.Clean` 仍位于目标库根路径之下。

### [P0-3] `/auth/fnos-login` 与 `/auth/fnos-reset-password` 未做鉴权、限流与来源校验
- **位置**：
  - [`server/server.go#L219-L221`](server/server.go#L219-L221)
  - [`server/auth.go#L53-L174`](server/auth.go#L53-L174)
- **原因与影响**：
  1. `/auth/fnos-login`（同时绑定 `POST` 和 `GET`）与 `/auth/fnos-reset-password` 均挂载在 `ClientIPRateLimiter` 作用域之外，没有任何 IP 白名单、`ND_AUTO_LOGIN` 开关判断或现有会话检查。
  2. `fnosLogin` 无条件采信客户端伪造的 `X-Fnos-User` / `X-Auth-User` / `Remote-User` / `X-Forwarded-User` 等 HTTP 头（完全绕过 `conf.Server.ExtAuth.TrustedSources` 检查），且当无请求头或查无此人时直接回落首个管理员，直接签发管理员 JWT 及 `subsonicSalt`/`subsonicToken`。
  3. `fnosResetPassword` 不验证旧密码或恢复口令，任何人只要能访问 4533 端口即可通过单次 POST 重置管理员密码并拿到有效管理员 Token。
- **修复建议**：
  1. 将 `/auth/fnos-login` 与 `/auth/fnos-reset-password` 受控于 `IsAutoLoginEnabled()` 且校验来源（仅允许 `127.0.0.1`/私有网段或已配置的可信反向代理 `TrustedSources`），并纳入登录限流组。
  2. `/auth/fnos-reset-password` 在系统已初始化后应要求验证当前管理员密码，或仅在首次初始化（无密码/明确 CLI 重置模式）时开放。

---

## 四、P1 高危问题（安全暴露、并发竞态、协议缺陷与核心功能 Bug）

### [P1-1] 公开分享页面 (`/share/*`) 注入默认管理员凭据及 URL Query 泄露 JWT
- **位置**：[`server/serve_index.go#L93-L99`](server/serve_index.go#L93-L99)、[`server/public/handle_shares.go#L42-L43`](server/public/handle_shares.go#L42-L43)、[`ui/src/eventStream.js#L11-L16`](ui/src/eventStream.js#L11-L16)、[`ui/src/onlinemusic/api.js#L121-L125`](ui/src/onlinemusic/api.js#L121-L125)
- **问题详情**：
  - 当 `IsAutoLoginEnabled()` 为真（默认开启）时，`serveIndex` 无条件向 `appConfig["auth"]` 注入管理员 JWT、用户名与 Subsonic 凭据。公开分享接口 `/share/{id}` 调用 `server.IndexWithShare(...)` 复用了同一函数，致使外部未登录访客打开公开分享链接即可从 HTML 内联 `window.__APP_CONFIG__` 提取管理员完整凭据。
  - 前端 SSE（`?jwt=`）与音源脚本下载 URL 将管理员 JWT 拼接在查询字符串中，易明文记录于反代与网关日志。
- **修复建议**：当 `shareInfo != nil` 时严禁执行 `handleDefaultAdminLogin`；鉴权优先通过 `Authorization` 头或 `HttpOnly Cookie` 传递，脚本下载改用已有的 `downloadSourceScript`（Header 鉴权）并移除 `getSourceDownloadUrl` 的 Query JWT 拼接。

### [P1-2] 脚本 ID 路径穿越及多处外部 HTTP `io.ReadAll` 无上限内存风险
- **位置**：[`core/musicengine/engine.go#L172`](core/musicengine/engine.go#L172), [`#L681-L733`](core/musicengine/engine.go#L681-L733), [`#L735-L741`](core/musicengine/engine.go#L735-L741), [`#L765-L785`](core/musicengine/engine.go#L765-L785)；[`core/musicengine/router.go#L109`](core/musicengine/router.go#L109)；[`core/musicengine/search.go#L462`](core/musicengine/search.go#L462)；[`core/musicengine/sandbox.go#L247`](core/musicengine/sandbox.go#L247), [`#L378`](core/musicengine/sandbox.go#L378)；[`core/output/dlna.go#L125`](core/output/dlna.go#L125), [`#L224`](core/output/dlna.go#L224)
- **问题详情**：`GetScriptFile`（`engine.go:739`）校验了 `validScriptIDPattern`，但 `ActivateScriptByID`、`DeleteScript`、`CheckUpdate` 直接用未经检查的 `scriptID` 执行 `filepath.Join` 读写/删除文件，存在目录穿越漏洞。此外多处 HTTP 响应与上传请求体直接使用 `io.ReadAll`（相比 `engine.go:672, 806` 的 10MB `LimitReader`），面对超大响应体可能触发 OOM。
- **修复建议**：抽取出统一的 `validateScriptID(scriptID)` 守卫所有脚本文件操作；为外部 HTTP 响应及表单读取统一套用合理上限的 `io.LimitReader`。

### [P1-3] `musicengine.StreamRelay` Seek 数据竞态、旧 Buffer 泄漏与双重 CDN 拉流
- **位置**：[`core/musicengine/relay.go#L127-L151`](core/musicengine/relay.go#L127-L151), [`#L180-L197`](core/musicengine/relay.go#L180-L197), [`#L249-L304`](core/musicengine/relay.go#L249-L304)；[`core/musicengine/router.go#L469-L474`](core/musicengine/router.go#L469-L474)；[`server/nativeapi/output.go#L160-L167`](server/nativeapi/output.go#L160-L167)；[`server/subsonic/stream.go#L725-L729`](server/subsonic/stream.go#L725-L729)
- **问题详情**：
  1. **竞态与内存泄漏**：触发不连续 Seek 时，`ServeHTTP` (`L186`) 在锁内直接 `r.buffer = NewSlabRingBuffer(reqStart)` 覆盖字段，既**没有对旧 Buffer 调用 `Close()`**（导致高达 32MB 的 slabs 无法归还池且旧读协程挂死），也没有在替换前停止上一条拉流协程。后台拉流协程（`L127-151`）直接读共享指针 `r.buffer` 而非局部变量快照，存在 Go 数据竞态，且可能把旧流残余字节写入新创建的 `r.buffer` 并错误触发 `MarkDone`。
  2. **双重 CDN 拉流**：三个上层路由均先调用 `relay.StartPull(ctx, 0)`，再调 `relay.ServeHTTP(w, r)`。当浏览器/DLNA/ffmpeg 发起 `Range: bytes=N-`（`N > 0`）请求时，`ServeHTTP` 在 `L183` 判定偏离并立刻掐断刚建好的 `offset=0` CDN 连接重新发起二次请求。
  3. **未拦截 `HEAD` 探测**：`ServeHTTP` 未针对 `req.Method == http.MethodHead` 早退，HQPlayer 常见的 `HEAD` 嗅探也会触发完整 CDN 下载、写 Body 与磁盘缓存流程。
- **修复建议**：将旧 buffer 赋值前先调用 `oldBuf.Close()`，拉流协程绑定局部变量 `buf := r.buffer`；由 `ServeHTTP` 在解析完 `Method` 与 `Range` 后按需首次触发 `StartPull(ctx, reqStart)`，且遇 `HEAD` 请求在写完响应头后直接返回。

### [P1-4] `SlabRingBuffer` 32MB 滑窗在高码率限速推流下断流 + `ReadAt` 等待协程泄漏
- **位置**：[`core/musicengine/buffer.go#L15-L19`](core/musicengine/buffer.go#L15-L19), [`#L176-L183`](core/musicengine/buffer.go#L176-L183), [`#L199-L202`](core/musicengine/buffer.go#L199-L202), [`#L237-L255`](core/musicengine/buffer.go#L237-L255)
- **问题详情**：
  1. `DefaultMaxSlabs = 512` 限制内存环形缓冲仅 **32MB**，且 `Write()` 不感知读者消费进度。CDN 满速下载，而后端用 `PacedResponseWriter`（`1.30x` 限速或 PCM `1.12x` 实况解码）缓慢供流时，任何体积 **> 32MB** 的在线无损 Hi-Res 音频都会使写指针覆盖淘汰未读旧分片，随即触发 `"read offset fallen behind sliding window"` 报错断流。
  2. `ReadAt`（`L237-243`）每次等待数据都会启动匿名 `go func() { b.mu.Lock(); b.cond.Wait(); ... }()`，如果 `30s` 超时（注意 `L236` 注释写的是 2.5s，实际 `ConditionWaitTime` 为 30s）触发返回后上游再无后续 `Broadcast`，该 goroutine 与关联内存会永久阻塞泄漏。
- **修复建议**：为 `SlabRingBuffer` 引入基于 `readCursor` 的背压机制（当 `writeCursor - minReaderOffset` 接近窗口上限时阻塞等待读者推进而非直接丢弃未读数据），并用非 `sync.Cond` 的 channel / timer 唤醒机制替代不可取消的 `go cond.Wait()`。

### [P1-5] `StreamCacheManager` 并发 `.tmp` 覆写与残缺下载提交为永久缓存
- **位置**：[`core/musicengine/relay.go#L257-L264`](core/musicengine/relay.go#L257-L264)、[`core/musicengine/cache.go#L173-L175`](core/musicengine/cache.go#L173-L175), [`#L239-L270`](core/musicengine/cache.go#L239-L270)
- **问题详情**：
  1. `relay.go:262` 条件 `commitCache(written >= streamLen || written > 1024*1024)` 意味着用户在 1MB 之后任意时机切歌或断网退出，这个仅下载了一半的残缺文件仍会被重命名为永久 `.stream` 缓存。此后该曲目无论在本机还是远端播放，都会命中残缺缓存并在半途截断。
  2. `cache.go` 对同一 `trackID` 使用固定文件名 `<md5>.tmp` 且 `os.OpenFile` 带 `O_TRUNC`（无 per-track 单飞/占用锁），两个客户端同时点播同一未缓存歌曲时会互相截断临时文件并在结束时竞态执行 `Rename`/`Remove`。
- **修复建议**：仅当真实接收完完整流（`written >= streamLen` 且无异常上下文取消/读错）时提交缓存；为临时文件使用唯一后缀（如 `<md5>.<rand>.tmp`）配合单曲并发下载去重。

### [P1-6] Goja `WorkerPool` 超时状态污染与跨协程异步 Panic（`fetch` / `setTimeout` / AES 截断）
- **位置**：[`core/musicengine/worker.go#L77-L90`](core/musicengine/worker.go#L77-L90)、[`core/musicengine/sandbox.go#L356-L462`](core/musicengine/sandbox.go#L356-L462), [`#L465-L556`](core/musicengine/sandbox.go#L465-L556)、[`core/musicengine/engine.go#L226-L278`](core/musicengine/engine.go#L226-L278)
- **问题详情**：
  1. **永久失效的池化 Worker**：`worker.go:85` 触发 `w.vm.Interrupt(...)` 后并未在返回 `sync.Pool` 前调用 `w.vm.ClearInterrupt()`（也未检查 `ctx.Done()`），一旦某次执行超时，归还池中的该 Worker 会永久处于中断态。
  2. **线程不安全调用 Panic**：Goja `*goja.Runtime` 严格禁止跨 Goroutine 访问，但 `sandbox.go` 的 `fetch` 后台协程（`L358-413`）与 `setTimeout` 的 `time.AfterFunc` 回调（`L440-445`）均直接异步操作 `vm` 与 JS 回调函数，极易引发并发 Panic。
  3. **异步 Promise 断链**：`engine.go:228-231` 与 `269-277` 期望 Promise 同步完成，没有 Goja 事件循环排空微任务；使用标准异步 `fetch` 的脚本会必现 `"script promise returned pending"`。
  4. **密码学兼容性缺陷**：`aesEncrypt` / `aesDecrypt`（`L466, 497`）固定使用 `padKey(key, 16)` 将 AES-192/256 密钥强截断为 AES-128；`pkcs7Unpad`（`L552-554`）验证异常时直接返回未剥离填充的原切片。
- **修复建议**：超时触发后丢弃或 `ClearInterrupt` + 重建 VM；引入单协程 Event Loop（或将 `fetch` 改为纯阻塞请求并将解析逻辑封送到唯一 VM 所属协程内履行 Promise）；按真实密钥长度（16/24/32 字节）选择 AES 块大小，严禁静默截断。

### [P1-7] `core/output/session.go` 与 `user_session.go` 多处锁外访问引发数据竞态
- **位置**：[`core/output/session.go#L178`](core/output/session.go#L178), [`#L207`](core/output/session.go#L207), [`#L243`](core/output/session.go#L243), [`#L260`](core/output/session.go#L260)；[`core/output/user_session.go#L344-L347`](core/output/user_session.go#L344-L347)
- **问题详情**：
  - `session.go` 的 `PlayTrackAt` (`L178`)、`Next` (`L207`)、`Prev` (`L243`) 在 `s.mu.Unlock()` 之后打日志时直接读取 `len(s.queue)`；`Pause` (`L260`) 在 `s.mu.Unlock()` 之后读取 `s.timeOffset`。
  - `user_session.go:344-347` 在持有外层 `UserSessionManager.mu` 期间直接修改 `sess.userName`，而 `UserSession` 内部快照与更新方法仅持有内层 `sess.mu`。以上均会在 `go test -race` 或高并发控制下触发数据竞态。
- **修复建议**：锁内提前拷贝局部变量 `qLen` / `pausedOffset` 用于后续日志打印；读写 `sess.userName` 统一使用 `sess.mu` 或原子字段。

### [P1-8] `server/events/sse.go` 移除写超时导致僵尸 SSE 连接永久挂死
- **位置**：[`server/events/sse.go#L143-L152`](server/events/sse.go#L143-L152)（对比上游移除 `writeTimeOut` 与 `setWriteTimeout`）
- **问题详情**：移除了每次 `writeEvent` 前的 `SetWriteDeadline` 后，在手机切后台休眠、Wi-Fi/4G 切换且 TCP 连接未收到 FIN/RST 的场景下，待内核发送缓冲区填满后，HTTP 写入将永久阻塞，导致僵尸 `ServeHTTP` 协程长期驻留无法退订。
- **修复建议**：保留面向 `http.ResponseController.SetWriteDeadline` 的超时保护（如配合 `keepAliveFrequency = 15s` 设置 `25s~30s` 写超时），既避免误杀正常运行客户端，又能及时清理死连接。

### [P1-9] `PlayerSession` 与 `UserSession` 双状态机分裂、`clear` 失效与队列模式缺陷
- **位置**：[`server/nativeapi/output.go#L287-L321`](server/nativeapi/output.go#L287-L321), [`output.go#L329`](server/nativeapi/output.go#L329), [`manager.go#L586-L588`](core/output/manager.go#L586-L588), [`output.go#L817-L876`](server/nativeapi/output.go#L817-L876)；[`core/output/session.go#L69-L81`](core/output/session.go#L69-L81), [`#L187-L196`](core/output/session.go#L187-L196), [`#L327-L331`](core/output/session.go#L327-L331), [`#L367-L406`](core/output/session.go#L367-L406)；[`core/output/manager.go#L698-L701`](core/output/manager.go#L698-L701)
- **问题详情**：
  1. **状态分裂与冗余重置广播**：`PlayerSession.onTrackEnded` 自动切歌后只触发 `mgr.NotifySessionChange()` 发送 SSE，未同步回写 `UserSession.remote`。当用户随后点击按钮触发 `POST /api/output/control` (`output.go:819`) 时，仍读取过时的 `sess.Snapshot("remote")`，且直接用 `req.TimeOffset`（暂停请求通常未携带 `timeOffset` -> 默认 `0`）覆写 `UserSession.remote` 并再广播一次 SSE，导致进度归零及图标/状态闪跳。
  2. **清空队列无法停止硬件**：`output.go:831-841` 处理 `action == "clear"` 时仅清空了 `UserSession` 便直接 `return`，未停止 `PlayerSession` 也没有停机释放 HQPlayer/DLNA；前端 `Player.jsx:1149` 遇 `queue.length === 0` 也直接返回，造成清空列表后设备继续播放。
  3. **播放模式与零时长死角**：
     - `nextTrackIndexLocked()`（`session.go:367-382`）仅处理 `"singleLoop"` 与 `"orderLoop"`，选择 `"shufflePlay"`（随机播放）实际按顺序播放并在最后一首直接停止；手动切下一曲 `Next(true)`（`L187-196`）亦忽略 `orderLoop`。
     - `armTimerLocked`（`L329-331`）在 `duration <= 0`（部分在线源/无时长标签文件）时直接返回，导致无法自动切歌和触发 HQPE 预排队。
     - `SetQueue` 的 P0 Guard（`L69-81`）或修改 `playMode`（`manager.go:698-701`）发生在最后 15 秒预入队（`<PlaylistAdd>`）之后时，HQPlayer 硬件内已挂载旧的下一曲目，到点会提前播错曲。
  4. **状态查询接口失真**：`GET /api/output/devices`（`output.go:329`）硬编码 `"activeId": "local"`，掩盖服务端真实活跃远端设备。
- **修复建议**：将 Remote 状态源收敛以 `PlayerSession` 为唯一真实源（或使其变更原子同步到 `UserSession.remote`）；补全 `"clear"` 停机清理、`"shufflePlay"` 随机调度与未知时长探活兜底；修正 `GET /api/output/devices` 返回真实设备 ID。

### [P1-10] `core/output/pcm_stream.go` 预估时长截断、HTTP Framing 破坏与非样本精确 Seek
- **位置**：[`core/output/pcm_stream.go#L140-L159`](core/output/pcm_stream.go#L140-L159), [`#L285-L294`](core/output/pcm_stream.go#L285-L294), [`#L350-L477`](core/output/pcm_stream.go#L350-L477), [`#L515-L547`](core/output/pcm_stream.go#L515-L547)
- **问题详情**：
  1. **HTTP `Content-Length` 与错误帧冲突**：`ServePCMStream` 在 `L285-294` 先行设置了巨大的音频 `Content-Length` / `Content-Range` 头，但后续遇到解析/转码/启动错误（如 `ffmpeg` 缺席、解码输出 0 字节等，`L350, 358, 381, 439, 446, 463, 475`）直接调用 `http.Error(...)` 而未删除 `Content-Length`，导致 HTTP 响应头声明长度与几十字节报错文本不匹配，触发播放器挂死或 `ERR_CONTENT_LENGTH_MISMATCH`。
  2. **估算字节数导致截尾或截流**：`ResolvePCMFormat`（`L140-159`）基于元数据 `mf.Duration`（无时长时回落 `240s`）提前计算 `TotalBytes`。真实解码 PCM 若长于估算，会被 `io.LimitReader(stdout, remaining)` 强行截尾；若比估算少超 5 秒（超过 `L534` 的 5s 静音补齐阈值），则会未写满 `Content-Length` 提前断流违背 HTTP 协议。
  3. **输入端近似 Seek 与在线 Relay 冲突**：`L420-422` 在压缩文件输入前使用 `-ss`（包/帧级近似定位），遇到 MP3/AAC/FLAC 断流 Range 重连会产生数十毫秒级的跳跃与错位；且未缓存网络歌曲使用的单例 `127.0.0.1` HTTP `relay`（`L377-398`）在遇到 `ffmpeg` 内部探测 Seek（如 M4A moov atom）时也会撞上 `[P1-3]` 的 Seek 竞态。
- **修复建议**：将易失败的前置检查与 WAV Header/FFmpeg 首块探测提至设置 `Content-Length` 之前，或在错误路径先 `w.Header().Del("Content-Length")` 与 `Content-Range`；通过 `ffprobe` 样本数或自适应静音填充修复长短差；对已打开的本地/缓存流考虑精确跳过已解码样本或保留足够冗余。

### [P1-11] HQPlayer `Empty transport` 重试耗尽仍返回成功 & DLNA SSDP 异步发现提前返回
- **位置**：[`core/output/hqplayer.go#L188-L210`](core/output/hqplayer.go#L188-L210)、[`core/output/dlna.go#L94-L107`](core/output/dlna.go#L94-L107)
- **问题详情**：
  1. `hqplayer.go:188-199` 中 `<Play/>` 若连续 12 次都遭遇 `"Empty transport"`，跳出循环后**未校验最后一次响应是否成功**，直接往下打印 `"HQPlayer XML: successfully switched and playing"` 并返回 `nil`，使后端错误维持 `playing` 假状态。
  2. `dlna.go:94-106` 在读取 SSDP UDP 回包期间异步 `go s.fetchDeviceDesc(loc)` 拉取设备 XML，但循环一结束就立即加读锁返回 `s.devices`，完全不等待异步 HTTP 请求完成。初次扫描或冷启动时极易返回空数组或缺失设备。
- **修复建议**：`hqplayer.go` 循环结束后判断若未收到 `result="OK"` 则返回错误；`dlna.go` 使用 `sync.WaitGroup` 在返回前汇合等待未完成的 XML 描述拉取协程。

### [P1-12] 前端播放器 (`Player.jsx`) 远端虚拟时钟双倍计时、拖动 Seek 与锁屏控制失效
- **位置**：[`ui/src/audioplayer/Player.jsx#L735-L745`](ui/src/audioplayer/Player.jsx#L735-L745), [`#L927-L934`](ui/src/audioplayer/Player.jsx#L927-L934), [`#L1282-L1287`](ui/src/audioplayer/Player.jsx#L1282-L1287), [`#L1350-L1354`](ui/src/audioplayer/Player.jsx#L1350-L1354), [`#L2330-L2391`](ui/src/audioplayer/Player.jsx#L2330-L2391), [`#L2398-L2466`](ui/src/audioplayer/Player.jsx#L2398-L2466), [`#L2531-L2545`](ui/src/audioplayer/Player.jsx#L2531-L2545)
- **问题详情**：
  1. **SSE 时间双倍叠加**：服务端 `UserSession` / `PlayerSession` 下发的 `timeOffset` 已经包含了 `(now - startedAt)` 的实时流逝时长，而 SSE 监听函数（`L736-739`）把 `baseClientTime` 设成历史时间戳 `remoteData.startedAt`（不同于 1 秒轮询在 `L1283` 使用的 `Date.now()`），导致虚拟时钟渲染（`L1353`）将已播时间**重复相加两次**，进度条在收到 SSE 瞬间暴涨（随后被 1 秒轮询拉回，形成来回跳变）。
  2. **远端 Seek 与锁屏 MediaSession 断联**：Remote 模式下 `<audio>` 加载的是 `SILENT_AUDIO_URI`（时长接近 0 秒）。进度条拖拽依赖 `audio.seeked`（`L2531-2545`），无法将真实百分比映射至目标秒数发送；`mediaSession` 的播放/暂停/上下曲/Seek 回调（`L2398-2466`）也只操控了本地静音 `<audio>`，未转发 `/api/output/control`。
  3. **设备切入按钮瞬态滞后**：`handleDeviceChanged`（`L927-934`）切入正在播放的 Remote 设备时仅手动派发一次 `play`/`pause` 事件，未复用 `syncRemoteUiIcon`（缺 `canplay` 与 80ms 补派机制），加载旋转图标清理不彻底。
  4. **快速失败卡死无跳过**：`handleError`（`L2330-2348`）在重锚定后若 500ms 内连续报解码错误直接置 `needsNetworkResumeRef = true` 提前退出，导致在有网络的情况下坏链音轨既不计次也不自动跳过。
- **修复建议**：SSE 处理与轮询统一令 `baseClientTime = Date.now()`；为 Remote 进度滑块挂接独立的松手目标计算并触发 `/api/output/control`，并令 `mediaSession` 根据当前模式路由至本地或远端接口；`handleDeviceChanged` 统一调用 `syncRemoteUiIcon`。

---

## 五、P2 中危问题（功能缺口、打包脚本与测试回归）

### [P2-1] `core/scrobbler/play_tracker.go` 单用户 Key 折叠破坏多终端独立播放记录
- **位置**：[`core/scrobbler/play_tracker.go#L286-L292`](core/scrobbler/play_tracker.go#L286-L292), [`#L469-L485`](core/scrobbler/play_tracker.go#L469-L485)
- **问题详情**：所有客户端均采用同一默认主管理员身份 (`user.ID` 相同)，但 `ReportPlayback` 强行用 `"user:" + user.ID` 作为 `playMap` 键（替代原生 `clientId`）。结果手机端 B 在 Local 模式切歌上报时，会直接覆盖并中断 PC 端 A 同账号的 `PlaybackSession`，导致多终端并行收听时 PlayCount / Last.fm Scrobble 统计错乱或丢失。
- **修复建议**：保留 `playMap` 按 `clientId`（如终端 `nd_client_unique_id` 或 `:remote` 通道）分键统计 Scrobble，仅在 `GetNowPlaying` 对外展示层若需聚合展示时做去重，避免破坏底层多端状态机。

### [P2-2] 聚合音源平台支持断层（`mg` / `qs` 失灵与兜底不可达）
- **位置**：[`ui/src/onlinemusic/OnlineMusicSearch.jsx#L225-L235`](ui/src/onlinemusic/OnlineMusicSearch.jsx#L225-L235)（及榜单/歌单页）；[`core/musicengine/search.go#L86-L92`](core/musicengine/search.go#L86-L92), [`#L554-L761`](core/musicengine/search.go#L554-L761)；[`core/musicengine/engine.go#L338-L345`](core/musicengine/engine.go#L338-L345)；[`core/musicengine/router.go#L290-L304`](core/musicengine/router.go#L290-L304), [`#L363-L376`](core/musicengine/router.go#L363-L376)
- **问题详情**：前端开放了咪咕（`mg`）与汽水（`qs`）数据源入口，但 `SearchNative` 遇到未知源即默认回落 `searchAll`（`kw, kg, tx, wy`）并返回混杂结果，致使 `engine.go:344` 的自定义 JS 音源搜索兜底分支永远无法触发；且 `GetSonglistDetailNative` 缺乏 `mg`/`qs` 实现，歌单/榜单详情点击后返回空列表。
- **修复建议**：未知/未原生实现的 `source` 应返回 `ErrUnsupportedNativeSource` 交由脚本池执行，或补齐对应平台的原生解析并裁剪无效 UI 选项。

### [P2-3] 多模块相对目录基准路径解析不统一
- **位置**：[`core/musicengine/paths.go#L21-L66`](core/musicengine/paths.go#L21-L66)、[`core/musicengine/cache.go#L51-L63`](core/musicengine/cache.go#L51-L63)、[`core/output/manager.go#L102-L113`](core/output/manager.go#L102-L113)、[`core/output/terminal_store.go#L42-L53`](core/output/terminal_store.go#L42-L53)、[`server/nativeapi/system_paths.go#L54-L61`](server/nativeapi/system_paths.go#L54-L61)
- **问题详情**：`paths.go` 将 `./var/...` 锚定到 `--configfile` 上两级并检查是否处于根文件系统，而 `cache.go`、`manager.go` 与 `terminal_store.go` 则直接取用 `conf.Server.DataFolder`/`CacheFolder` 并在为空时才读 `TRIM_PKGVAR`。当从非套件根目录直接手工启动 `--configfile` 传入相对路径配置时，各模块会分别落盘到不同的位置。
- **修复建议**：收敛抽取统一的存储路径锚定 Helper，所有状态文件与缓存子目录均复用一致的根目录解析规则。

### [P2-4] 飞牛 `.fnos` 打包清单、CI 构建脚本与卸载逻辑存在多处错配
- **位置**：
  - [`.fnos/manifest.json#L3`](.fnos/manifest.json#L3) vs [`.fnos/manifest#L2`](.fnos/manifest#L2)
  - [`.github/workflows/build-fpk.yml#L71-L74`](.github/workflows/build-fpk.yml#L71-L74), [`#L90`](.github/workflows/build-fpk.yml#L90) vs [`.fnos/ui/config#L3`](.fnos/ui/config#L3)
  - [`.fnos/cmd/uninstall_callback#L20`](.fnos/cmd/uninstall_callback#L20)
  - [`.fnos/cmd/main#L15`](.fnos/cmd/main#L15), [`#L59-L60`](.fnos/cmd/main#L59-L60)
- **问题详情**：
  1. `.fnos/manifest.json` 版本号停留在 `"0.64.3"`，未随 `.fnos/manifest` 同步更新至 `0.64.33`。
  2. `build-fpk.yml:74` 用 `sed -i 's|description = .*|...'` 修改 Debug FPK 描述，但 `.fnos/manifest:4` 的字段键名是 `desc =` 而非 `description =`，导致替换静默失效。
  3. 仓库内 `.fnos/ui/config` 与 `.fnos/config/config.json` 注册启动名为 `navidrome`，而 CI 打包过程中又将其覆写为 `navidrome.Application`，本地手工封包与 CI 产物行为不一致。
  4. `uninstall_callback:20` 将 `"${TRIM_PKGVAR}/*.log"` 和 `"${TRIM_PKGVAR}/*.pid"` 放进双引号内，致使 Shell 通配符不展开，卸载清理数据时日志与 PID 文件始终残留。
  5. `.fnos/cmd/main:15` 每次启动对持久化目录递归执行 `chmod -R 777` 存在安全隐患且大缓存目录耗时随数据增长；`L59` 用 `bash -c "${CMD}" ... &` 且未带 `exec`，`app.pid` 仅记录外层外壳 Shell 的 PID。
- **修复建议**：统一版本与 UI 配置键名；修正 CI `sed` 正则匹配 `desc =`；移除通配符外双引号（`"${TRIM_PKGVAR}"/*.log`）；改用 `exec` 启动并缩小权限范围至 `0755`/`0750`。

### [P2-5] 前端单测回归失败未同步修复
- **位置**：[`ui/src/audioplayer/PlayerToolbar.jsx#L118-L126`](ui/src/audioplayer/PlayerToolbar.jsx#L118-L126) vs [`PlayerToolbar.test.jsx`](ui/src/audioplayer/PlayerToolbar.test.jsx)；[`ui/src/eventStream.js#L60`](ui/src/eventStream.js#L60) vs [`eventStream.test.js`](ui/src/eventStream.test.js)
- **问题详情**：移动端播放器工具栏裁剪了收藏与保存队列按钮、`eventStream` 将 `serverDown()` 触发门槛调整为连续 3 次错误后，均未同步更新对应的 Vitest 单元测试，导致现有测试套件报错红灯。
- **修复建议**：明确上述 UI/重连行为为预期设计后，补全/更新对应断言；并排查其余 5 个测试文件在全量测试中的超时及失败原因。

---

## 六、P3 优化建议与工程卫生

1. **避免每次请求分配 3MB 缓冲切片（[`core/stream/media_streamer.go#L325`](core/stream/media_streamer.go#L325)）**：`burstResponseWriter.ReadFrom` 在每次调用都会 `make([]byte, 3*1024*1024)`。即使是几十字节的小分片 `Range` 探测请求也会分配 3MB 内存并带来显著 GC 压力；建议改用 `sync.Pool` 复用或流式按小分片拷贝首 3MB 数据。
2. **补充 `PacedResponseWriter.Unwrap()`（[`core/stream/pacer.go#L15-L152`](core/stream/pacer.go#L15-L152)）**：增加 `Unwrap() http.ResponseWriter` 以兼容 Go 1.20+ 标准库 `http.ResponseController`。
3. **JSON 配置/队列持久化序列化加锁与增量写入（[`core/output/terminal_store.go#L69-L82`](core/output/terminal_store.go#L69-L82)、[`core/output/manager.go#L148-L170`](core/output/manager.go#L148-L170)）**：两处均通过异步 `go save()` 无文件锁写文件（且 `manager.saveState()` 未写 `.tmp` 而是直接 `os.WriteFile`，`terminal_store.save()` 并发高频切歌会竞争同一个 `.tmp` 临时文件导致损坏或 `Rename` 失败）；建议加串行写入锁或直接迁入 SQLite。
4. **清理死代码与未闭环符号**：
   - [`core/musicengine/shadow.go#L160`](core/musicengine/shadow.go#L160) `UpdateRealMetadata` 定义后从未被任何调用方触发，建议在首次拉流嗅探 / `ResolvePCMFormat` 探针得到真实大小、采样率、位深后按需异步调用，消除在线歌曲默认估算文件大小导致的各种进度与流长偏差。
   - [`ui/src/audioplayer/HQPlayerTakeoverDialog.jsx`](ui/src/audioplayer/HQPlayerTakeoverDialog.jsx)（单主账号模式已取消 409 冲突弹窗）、[`ui/src/eventStream.js#L108`](ui/src/eventStream.js#L108) `startEventStreamLegacy` 属于未引用遗留代码，建议确认无用后清理。
5. **收窄全局 `unhandledrejection` 抑制并模块化重构 `Player.jsx`**：
   - [`ui/index.html#L168-L177`](ui/index.html#L168-L177) 吞掉所有含 `"abort"` 字样的异常与所有 `NotSupportedError`，且 `/vconsole.min.js`（`L86`）未适配非根 `BasePath`（若部署子路径会加载失败）；建议精准匹配 `DOMException` 实例名并使用相对/基础路径。
   - [`ui/src/audioplayer/Player.jsx`](ui/src/audioplayer/Player.jsx) 单文件超过 2700 行且混合了 `document.querySelectorAll` 直接操作 DOM（与 React 渲染竞争），建议按职责拆分为 `useLocalAudioGuard`、`useRemotePlayerSync`、`useMediaSessionBridge` 等独立 Hook，提升可维护性与可测性。
