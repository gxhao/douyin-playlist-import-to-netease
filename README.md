## 起因
因为想找抖音的一些BGM,所以下载了汽水音乐，用起来体验还可以，但是十几天后vip到期了，而且我没有续期的意愿，所以想把抖音的收藏的歌曲导入网易云。
## 在线工具
https://www.tunemymusic.com/zh-CN/transfer
支持的是一些国外的，缺少国内的软件
## 执行计划
# Import Douyin Playlist to NetEase Cloud Music - Implementation Plan

## Goal Description
Create a Go program `importneteasemuscic.go` that takes a Douyin/Qishui playlist URL, parses the song list (titles and artists), and imports them into a specified NetEase Cloud Music playlist.

## User Review Required
- [ ] **NetEase Login Method**: The tool will likely need to use QR code login or cookie input. We will implement QR code login if a suitable library is found, or ask for cookies.
- [ ] **NetEase Library**: We will use a Go library for NetEase API. `github.com/chaunsin/netease-cloud-music` or similar.

## Proposed Changes

### Project Structure
- `importneteasemuscic.go`: Main entry point.
- `go.mod`: Dependency management.

### Component: Douyin Parsing
- **Function**: `ParseDouyinPlaylist(url string) ([]Song, error)`
- **Logic**:
    1. Resolve redirect to get the `music.douyin.com` URL.
    2. Fetch the HTML content.
    3. Extract `_ROUTER_DATA` JSON from the `<script>` tag.
    4. Parse the JSON to extracting song `name` and `artists`.

### Component: NetEase Interaction
- **Function**: `LoginNetEase() (*api.Client, *weapi.Api, error)`
    - Returns `*api.Client` for searching and `*weapi.Api` for playlist operations.
- **Function**: `SearchSong(client *api.Client, query string) (int64, error)`
    - Uses `client.Request` to hit `/weapi/cloudsearch/get/web`.
- **Function**: `AddSongToPlaylist(wapi *weapi.Api, playlistID int64, songID int64) error`
    - Uses `wapi.PlaylistAddOrDel`.

## Verification Plan
### Automated Tests
- Run `go run importneteasemuscic.go` with the provided Douyin URL.
- Verify that song titles are printed to the console.
- (Later) Verify songs are added to NetEase.

<img width="628" height="566" alt="3ca2c5efc77328123fb43900520259b6" src="https://github.com/user-attachments/assets/b5832d6e-b66d-4979-93ba-0a1e097cb557" />

