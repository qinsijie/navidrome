package musicengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{
	Timeout: 8 * time.Second,
}

// SearchNative queries music from upstream APIs natively
func SearchNative(ctx context.Context, source, query string, page, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 30
	}
	if page <= 0 {
		page = 1
	}

	result := &SearchResult{
		Source: source,
		Page:   page,
		Limit:  limit,
		List:   make([]*OnlineTrack, 0),
	}

	if strings.TrimSpace(query) == "" {
		return result, nil
	}

	switch strings.ToLower(source) {
	case "wy":
		list, err := searchNetEase(ctx, query, page, limit)
		if err == nil {
			result.List = list
			result.Total = len(list)
		}
		return result, err
	case "kw":
		list, err := searchKuwo(ctx, query, page, limit)
		if err == nil {
			result.List = list
			result.Total = len(list)
		}
		return result, err
	case "kg":
		list, err := searchKuGou(ctx, query, page, limit)
		if err == nil {
			result.List = list
			result.Total = len(list)
		}
		return result, err
	case "tx":
		list, err := searchQQ(ctx, query, page, limit)
		if err == nil {
			result.List = list
			result.Total = len(list)
		}
		return result, err
	case "all", "":
		// Aggregated search across kw, kg, tx, wy concurrently
		list := searchAll(ctx, query, page, limit)
		result.Source = "all"
		result.List = list
		result.Total = len(list)
		return result, nil
	default:
		// Default to NetEase or All
		list := searchAll(ctx, query, page, limit)
		result.List = list
		result.Total = len(list)
		return result, nil
	}
}

// searchAll runs multi-source search in parallel and interleaves the results
func searchAll(ctx context.Context, query string, page, limit int) []*OnlineTrack {
	sources := []string{"kw", "kg", "tx", "wy"}
	type fetchRes struct {
		src  string
		list []*OnlineTrack
	}

	ch := make(chan fetchRes, len(sources))
	var wg sync.WaitGroup

	fetchLimit := limit
	if fetchLimit < 15 {
		fetchLimit = 15
	}

	for _, src := range sources {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			var l []*OnlineTrack
			var err error
			switch s {
			case "kw":
				l, err = searchKuwo(ctx, query, page, fetchLimit)
			case "kg":
				l, err = searchKuGou(ctx, query, page, fetchLimit)
			case "tx":
				l, err = searchQQ(ctx, query, page, fetchLimit)
			case "wy":
				l, err = searchNetEase(ctx, query, page, fetchLimit)
			}
			if err == nil && len(l) > 0 {
				ch <- fetchRes{src: s, list: l}
			}
		}(src)
	}

	wg.Wait()
	close(ch)

	resultsMap := make(map[string][]*OnlineTrack)
	for r := range ch {
		resultsMap[r.src] = r.list
	}

	// Interleave results in round-robin order: kw, kg, tx, wy
	var merged []*OnlineTrack
	seenKeys := make(map[string]bool)

	maxLen := 0
	for _, l := range resultsMap {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}

	for i := 0; i < maxLen && len(merged) < limit; i++ {
		for _, s := range sources {
			list := resultsMap[s]
			if i < len(list) {
				t := list[i]
				dedupKey := strings.ToLower(fmt.Sprintf("%s-%s", t.Title, t.Artist))
				if !seenKeys[dedupKey] {
					seenKeys[dedupKey] = true
					merged = append(merged, t)
				}
				if len(merged) >= limit {
					break
				}
			}
		}
	}

	return merged
}

// searchNetEase searches NetEase Cloud Music
func searchNetEase(ctx context.Context, query string, page, limit int) ([]*OnlineTrack, error) {
	offset := (page - 1) * limit
	apiURL := fmt.Sprintf("http://music.163.com/api/search/get/web?s=%s&type=1&offset=%d&limit=%d",
		url.QueryEscape(query), offset, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Referer", "http://music.163.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Result struct {
			Songs []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Duration int64  `json:"duration"`
				Artists  []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					PicURL string `json:"picUrl"`
				} `json:"album"`
			} `json:"songs"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var tracks []*OnlineTrack
	for _, s := range data.Result.Songs {
		var artists []string
		for _, a := range s.Artists {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		artistStr := strings.Join(artists, " / ")
		songID := strconv.FormatInt(s.ID, 10)

		tracks = append(tracks, &OnlineTrack{
			UniversalID:  BuildUniversalID("wy", songID),
			ProtocolPath: BuildProtocolPath("wy", songID, QualityFlac),
			Source:       "wy",
			SongID:       songID,
			Title:        s.Name,
			Artist:       artistStr,
			Album:        s.Album.Name,
			Duration:     float64(s.Duration) / 1000.0,
			CoverURL:     s.Album.PicURL,
			Quality:      QualityFlac,
			Qualities:    []string{Quality128k, Quality320k, QualityFlac},
		})
	}

	return tracks, nil
}

// searchKuwo searches Kuwo Music
func searchKuwo(ctx context.Context, query string, page, limit int) ([]*OnlineTrack, error) {
	apiURL := fmt.Sprintf("http://search.kuwo.cn/r.s?client=kt&all=%s&pn=%d&rn=%d&uid=794762570&ver=kwplayer_ar_9.2.2.1&vipver=1&show_copyright_off=1&newver=1&ft=music&cluster=0&strategy=2012&encoding=utf8&rformat=json&vermerge=1&mobi=1&issubtitle=1",
		url.QueryEscape(query), page-1, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Abslist []struct {
			MUSICRID string `json:"MUSICRID"`
			SONGNAME string `json:"SONGNAME"`
			ARTIST   string `json:"ARTIST"`
			ALBUM    string `json:"ALBUM"`
			DURATION string `json:"DURATION"`
			NMINFO   string `json:"N_MINFO"`
		} `json:"abslist"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var tracks []*OnlineTrack
	for _, item := range data.Abslist {
		songID := strings.TrimPrefix(item.MUSICRID, "MUSIC_")
		dur, _ := strconv.ParseFloat(item.DURATION, 64)

		qualities := []string{Quality128k, Quality320k, QualityFlac}
		if strings.Contains(item.NMINFO, "flac24bit") || strings.Contains(item.NMINFO, "4000") {
			qualities = append(qualities, QualityFlac24b)
		}

		tracks = append(tracks, &OnlineTrack{
			UniversalID:  BuildUniversalID("kw", songID),
			ProtocolPath: BuildProtocolPath("kw", songID, QualityFlac),
			Source:       "kw",
			SongID:       songID,
			Title:        item.SONGNAME,
			Artist:       item.ARTIST,
			Album:        item.ALBUM,
			Duration:     dur,
			Quality:      QualityFlac,
			Qualities:    qualities,
		})
	}

	return tracks, nil
}

// searchKuGou searches KuGou Music
func searchKuGou(ctx context.Context, query string, page, limit int) ([]*OnlineTrack, error) {
	apiURL := fmt.Sprintf("https://songsearch.kugou.com/song_search_v2?keyword=%s&page=%d&pagesize=%d&userid=0&clientver=&platform=WebFilter&filter=2&iscorrection=1&privilege_filter=0&area_code=1",
		url.QueryEscape(query), page, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			Lists []struct {
				FileHash   string `json:"FileHash"`
				HQFileHash string `json:"HQFileHash"`
				SQFileHash string `json:"SQFileHash"`
				SongName   string `json:"SongName"`
				SingerName string `json:"SingerName"`
				AlbumName  string `json:"AlbumName"`
				Duration   int    `json:"Duration"`
			} `json:"lists"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var tracks []*OnlineTrack
	for _, item := range data.Data.Lists {
		hash := item.FileHash
		if item.SQFileHash != "" {
			hash = item.SQFileHash
		} else if item.HQFileHash != "" {
			hash = item.HQFileHash
		}
		if hash == "" {
			continue
		}

		qualities := []string{Quality128k, Quality320k}
		if item.SQFileHash != "" {
			qualities = append(qualities, QualityFlac)
		}

		tracks = append(tracks, &OnlineTrack{
			UniversalID:  BuildUniversalID("kg", hash),
			ProtocolPath: BuildProtocolPath("kg", hash, QualityFlac),
			Source:       "kg",
			SongID:       hash,
			Title:        item.SongName,
			Artist:       item.SingerName,
			Album:        item.AlbumName,
			Duration:     float64(item.Duration),
			Quality:      QualityFlac,
			Qualities:    qualities,
		})
	}

	return tracks, nil
}

// searchQQ searches QQ Music
func searchQQ(ctx context.Context, query string, page, limit int) ([]*OnlineTrack, error) {
	apiURL := fmt.Sprintf("https://c.y.qq.com/soso/fcgi-bin/client_search_cp?p=%d&n=%d&w=%s&format=json",
		page, limit, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Referer", "https://y.qq.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			Song struct {
				List []struct {
					Songmid   string `json:"songmid"`
					Songname  string `json:"songname"`
					Albumname string `json:"albumname"`
					Interval  int    `json:"interval"`
					Singer    []struct {
						Name string `json:"name"`
					} `json:"singer"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var tracks []*OnlineTrack
	for _, item := range data.Data.Song.List {
		var singers []string
		for _, s := range item.Singer {
			if s.Name != "" {
				singers = append(singers, s.Name)
			}
		}
		artistStr := strings.Join(singers, " / ")

		tracks = append(tracks, &OnlineTrack{
			UniversalID:  BuildUniversalID("tx", item.Songmid),
			ProtocolPath: BuildProtocolPath("tx", item.Songmid, QualityFlac),
			Source:       "tx",
			SongID:       item.Songmid,
			Title:        item.Songname,
			Artist:       artistStr,
			Album:        item.Albumname,
			Duration:     float64(item.Interval),
			Quality:      QualityFlac,
			Qualities:    []string{Quality128k, Quality320k, QualityFlac},
		})
	}

	return tracks, nil
}

// GetSearchTips queries real-time autocomplete suggestions from Kuwo suggestions API
func GetSearchTips(ctx context.Context, query string) ([]string, error) {
	if strings.TrimSpace(query) == "" {
		return []string{}, nil
	}

	apiURL := fmt.Sprintf("https://tips.kuwo.cn/t.s?corp=kuwo&newver=3&p2p=1&notrace=0&c=mbox&w=%s&encoding=utf8&rformat=json",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Referer", "http://www.kuwo.cn/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		WordItems []struct {
			RelWord string `json:"RELWORD"`
		} `json:"WORDITEMS"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return []string{}, nil
	}

	var tips []string
	for _, item := range data.WordItems {
		if item.RelWord != "" {
			tips = append(tips, item.RelWord)
		}
	}

	return tips, nil
}

// GetSonglistDetailNative fetches playlist details from upstream
func GetSonglistDetailNative(ctx context.Context, source, id string) (*SongListDetail, error) {
	detail := &SongListDetail{
		ID:     id,
		Source: source,
		Title:  "网络精选歌单",
		List:   make([]*OnlineTrack, 0),
	}

	if strings.ToLower(source) == "wy" {
		apiURL := fmt.Sprintf("http://music.163.com/api/playlist/detail?id=%s", url.QueryEscape(id))
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return detail, err
		}
		req.Header.Set("Referer", "http://music.163.com")

		resp, err := httpClient.Do(req)
		if err != nil {
			return detail, err
		}
		defer resp.Body.Close()

		var data struct {
			Result struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				CoverImgURL string `json:"coverImgUrl"`
				Tracks      []struct {
					ID       int64  `json:"id"`
					Name     string `json:"name"`
					Duration int64  `json:"duration"`
					Artists  []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name   string `json:"name"`
						PicURL string `json:"picUrl"`
					} `json:"album"`
				} `json:"tracks"`
			} `json:"result"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if data.Result.Name != "" {
				detail.Title = data.Result.Name
			}
			detail.Desc = data.Result.Description
			detail.CoverURL = data.Result.CoverImgURL

			for _, t := range data.Result.Tracks {
				var artists []string
				for _, a := range t.Artists {
					if a.Name != "" {
						artists = append(artists, a.Name)
					}
				}
				songID := strconv.FormatInt(t.ID, 10)
				detail.List = append(detail.List, &OnlineTrack{
					UniversalID:  BuildUniversalID("wy", songID),
					ProtocolPath: BuildProtocolPath("wy", songID, QualityFlac),
					Source:       "wy",
					SongID:       songID,
					Title:        t.Name,
					Artist:       strings.Join(artists, " / "),
					Album:        t.Album.Name,
					Duration:     float64(t.Duration) / 1000.0,
					CoverURL:     t.Album.PicURL,
					Quality:      QualityFlac,
					Qualities:    []string{Quality128k, Quality320k, QualityFlac},
				})
			}
		}
	} else if strings.ToLower(source) == "kw" {
		apiURL := fmt.Sprintf("http://kbangserver.kuwo.cn/ksong.s?from=pc&fmt=json&type=bang&data=content&id=%s&pn=0&rn=100", url.QueryEscape(id))
		if len(id) > 5 {
			apiURL = fmt.Sprintf("http://nplserver.kuwo.cn/pl.svc?op=getlistinfo&pid=%s&pn=0&rn=100&encode=utf8&keyset=pl2012&identity=kuwo&pcmp4=1&vipver=MUSIC_9.0.5.0_W1&newver=1", url.QueryEscape(id))
		}
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return detail, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return detail, err
		}
		defer resp.Body.Close()

		var data struct {
			Title     string `json:"title"`
			Name      string `json:"name"`
			Info      string `json:"info"`
			Pic       string `json:"pic"`
			Musiclist []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				Artist       string `json:"artist"`
				Album        string `json:"album"`
				Duration     string `json:"duration"`
				SongDuration string `json:"song_duration"`
			} `json:"musiclist"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if data.Title != "" {
				detail.Title = data.Title
			} else if data.Name != "" {
				detail.Title = data.Name
			}
			detail.Desc = data.Info
			detail.CoverURL = data.Pic

			for _, m := range data.Musiclist {
				dur, _ := strconv.ParseFloat(m.SongDuration, 64)
				if dur == 0 {
					dur, _ = strconv.ParseFloat(m.Duration, 64)
				}
				detail.List = append(detail.List, &OnlineTrack{
					UniversalID:  BuildUniversalID("kw", m.ID),
					ProtocolPath: BuildProtocolPath("kw", m.ID, QualityFlac),
					Source:       "kw",
					SongID:       m.ID,
					Title:        m.Name,
					Artist:       m.Artist,
					Album:        m.Album,
					Duration:     dur,
					Quality:      QualityFlac,
					Qualities:    []string{Quality128k, Quality320k, QualityFlac},
				})
			}
		}
	} else if strings.ToLower(source) == "kg" {
		apiURL := fmt.Sprintf("http://m.kugou.com/rank/info/?rankid=%s&page=1&json=true", url.QueryEscape(id))
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return detail, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return detail, err
		}
		defer resp.Body.Close()

		var data struct {
			Info struct {
				Rankname string `json:"rankname"`
				Intro    string `json:"intro"`
				Imgurl   string `json:"imgurl"`
			} `json:"info"`
			Songs struct {
				List []struct {
					Hash     string `json:"hash"`
					Sqhash   string `json:"sqhash"`
					Songname string `json:"songname"`
					Filename string `json:"filename"`
					Duration int64  `json:"duration"`
					Authors  []struct {
						AuthorName string `json:"author_name"`
					} `json:"authors"`
					AlbumSizableCover string `json:"album_sizable_cover"`
				} `json:"list"`
			} `json:"songs"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if data.Info.Rankname != "" {
				detail.Title = data.Info.Rankname
			}
			detail.Desc = data.Info.Intro
			detail.CoverURL = strings.Replace(data.Info.Imgurl, "{size}", "400", -1)

			for _, s := range data.Songs.List {
				songID := s.Hash
				if songID == "" {
					songID = s.Sqhash
				}
				var artists []string
				for _, a := range s.Authors {
					if a.AuthorName != "" {
						artists = append(artists, a.AuthorName)
					}
				}
				artistName := strings.Join(artists, " / ")
				title := s.Songname
				if title == "" && strings.Contains(s.Filename, " - ") {
					parts := strings.SplitN(s.Filename, " - ", 2)
					if artistName == "" {
						artistName = strings.TrimSpace(parts[0])
					}
					title = strings.TrimSpace(parts[1])
				}
				cover := strings.Replace(s.AlbumSizableCover, "{size}", "400", -1)

				detail.List = append(detail.List, &OnlineTrack{
					UniversalID:  BuildUniversalID("kg", songID),
					ProtocolPath: BuildProtocolPath("kg", songID, QualityFlac),
					Source:       "kg",
					SongID:       songID,
					Title:        title,
					Artist:       artistName,
					Duration:     float64(s.Duration),
					CoverURL:     cover,
					Quality:      QualityFlac,
					Qualities:    []string{Quality128k, Quality320k, QualityFlac},
				})
			}
		}
	} else if strings.ToLower(source) == "tx" {
		apiURL := fmt.Sprintf("https://c.y.qq.com/v8/fcg-bin/fcg_v8_toplist_cp.fcg?topid=%s&format=json", url.QueryEscape(id))
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return detail, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return detail, err
		}
		defer resp.Body.Close()

		var data struct {
			Topinfo struct {
				ListName string `json:"ListName"`
				Info     string `json:"info"`
				Pic      string `json:"pic"`
				PicAlbum string `json:"pic_album"`
			} `json:"topinfo"`
			Songlist []struct {
				Data struct {
					Songmid  string `json:"songmid"`
					Songname string `json:"songname"`
					Interval int64  `json:"interval"`
					Singer   []struct {
						Name string `json:"name"`
					} `json:"singer"`
					Albumname string `json:"albumname"`
					Albummid  string `json:"albummid"`
				} `json:"data"`
			} `json:"songlist"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if data.Topinfo.ListName != "" {
				detail.Title = data.Topinfo.ListName
			}
			detail.Desc = data.Topinfo.Info
			detail.CoverURL = data.Topinfo.Pic
			if detail.CoverURL == "" {
				detail.CoverURL = data.Topinfo.PicAlbum
			}

			for _, item := range data.Songlist {
				t := item.Data
				var singers []string
				for _, s := range t.Singer {
					if s.Name != "" {
						singers = append(singers, s.Name)
					}
				}
				cover := ""
				if t.Albummid != "" {
					cover = fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T002R300x300M000%s.jpg", t.Albummid)
				}
				detail.List = append(detail.List, &OnlineTrack{
					UniversalID:  BuildUniversalID("tx", t.Songmid),
					ProtocolPath: BuildProtocolPath("tx", t.Songmid, QualityFlac),
					Source:       "tx",
					SongID:       t.Songmid,
					Title:        t.Songname,
					Artist:       strings.Join(singers, " / "),
					Album:        t.Albumname,
					Duration:     float64(t.Interval),
					CoverURL:     cover,
					Quality:      QualityFlac,
					Qualities:    []string{Quality128k, Quality320k, QualityFlac},
				})
			}
		}
	}

	return detail, nil
}
