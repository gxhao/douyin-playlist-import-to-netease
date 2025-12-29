package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"


	"github.com/chaunsin/netease-cloud-music/api"
	"github.com/chaunsin/netease-cloud-music/api/types"
	"github.com/chaunsin/netease-cloud-music/api/weapi"
	nlog "github.com/chaunsin/netease-cloud-music/pkg/log"
)

type RouterData struct {
	LoaderData struct {
		PlaylistPage struct {
			Medias []struct {
				Type   string `json:"type"`
				Entity struct {
					Track struct {
						Name string `json:"name"`
                        Artists []struct {
                            Name string `json:"name"`
                        } `json:"artists"`
					} `json:"track"`
				} `json:"entity"`
			} `json:"medias"`
		} `json:"playlist_page"`
	} `json:"loaderData"`
}

func main() {
	// 1. Login to NetEase
	client, wapi, err := LoginNetEase()
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	fmt.Println("NetEase Login flow completed.")

	// 2. Parse Douyin Playlist
    reader := bufio.NewReader(os.Stdin)
    fmt.Print("Enter Douyin Playlist URL (e.g., https://qishui.douyin.com/s/iHpcChBN/): ")
    douyinURL, _ := reader.ReadString('\n')
    douyinURL = strings.TrimSpace(douyinURL)
    
    if douyinURL == "" {
        fmt.Println("Using default URL for testing...")
        douyinURL = "https://qishui.douyin.com/s/iHpcChBN/"
    }

	songs, err := ParseDouyinPlaylist(douyinURL)
	if err != nil {
		fmt.Printf("Error parsing Douyin playlist: %v\n", err)
		return
	}

	fmt.Printf("Found %d songs from Douyin.\n", len(songs))

    // 3. Get Target Playlist ID (Ask User)
    // reader is already created
    fmt.Print("Enter NetEase Playlist ID to import to: ")
    playlistIDStr, _ := reader.ReadString('\n')
    playlistIDStr = strings.TrimSpace(playlistIDStr)
    var playlistID int64
    fmt.Sscanf(playlistIDStr, "%d", &playlistID)

    if playlistID == 0 {
        fmt.Println("Invalid playlist ID.")
        return
    }

    // 4. Import Songs
    var successCount, failCount int
    for i, song := range songs {
        fmt.Printf("[%d/%d] Processing: %s - %s ... ", i+1, len(songs), song.Title, song.Artist)
        
        // Search
        songID, err := SearchSong(client, song.Title, song.Artist)
        if err != nil {
            fmt.Printf("Search failed: %v\n", err)
            failCount++
            continue
        }
        if songID == 0 {
            fmt.Println("Not found.")
            failCount++
            continue
        }

        // Add
        err = AddSongToPlaylist(wapi, playlistID, songID)
        if err != nil {
             // Check if it's already in playlist (simplified check)
             if strings.Contains(err.Error(), "502") {
                 fmt.Println("Already in playlist.")
                 successCount++ // Count as success
             } else {
                 fmt.Printf("Add failed: %v\n", err)
                 failCount++
             }
        } else {
            fmt.Println("Added.")
            successCount++
        }

        // Rate limit
        time.Sleep(1 * time.Second)
    }
    
    fmt.Printf("Import completed. Success: %d, Failed: %d\n", successCount, failCount)
}

type Song struct {
	Title  string
	Artist string
}

func LoginNetEase() (*api.Client, *weapi.Api, error) {
    // 1. Initialize API client
    // Initialize logger
    logCfg := &nlog.Config{
        Stdout: true,
        Level:  "info",
    }
    logger := nlog.New(logCfg)
    nlog.Default = logger // Set global default logger as library uses it
    
    // Use default api config
    cfg := &api.Config{}
    
    // api.NewClient(cfg, logger)
    client, err := api.NewClient(cfg, logger)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create api client: %v", err)
    }
    
    wapi := weapi.New(client)

    // 2. Create QR Code Key
    // weapi.QrcodeCreateKey
    keyResp, err := wapi.QrcodeCreateKey(context.Background(), &weapi.QrcodeCreateKeyReq{Type: 1})
    if err != nil {
        return nil, nil, fmt.Errorf("create key failed: %v", err)
    }
    unikey := keyResp.UniKey
    // fmt.Println("QR Code Key:", unikey)

    // 3. Generate QR Code
    qrResp, err := wapi.QrcodeGenerate(context.Background(), &weapi.QrcodeGenerateReq{CodeKey: unikey})
    if err != nil {
        return nil, nil, fmt.Errorf("generate qr failed: %v", err)
    }
    // Check struct fields
    fmt.Printf("Please scan the QR code:\n%s\n", qrResp.QrcodePrint)
    // fmt.Printf("QR Code (Bytes): %v\n", qrResp.Qrcode)
    
    // 4. Poll for status
    fmt.Println("Waiting for scan...")
    for {
        time.Sleep(2 * time.Second)
        checkResp, err := wapi.QrcodeCheck(context.Background(), &weapi.QrcodeCheckReq{Key: unikey, Type: 1})
        if err != nil {
            fmt.Printf("Check error: %v\n", err)
            continue 
        }

        code := checkResp.Code
        // 800: Expired, 801: Waiting, 802: Scanned, 803: Success
        if code == 800 {
            return nil, nil, fmt.Errorf("qr code expired")
        } else if code == 803 {
            fmt.Println("Login successful!")
            return client, wapi, nil
        } else if code == 801 {
            // Waiting
        } else if code == 802 {
            fmt.Println("Scanned! waiting for confirmation...")
        }
    }
}

type SearchResp struct {
    Result struct {
        Songs []struct {
            Id int64 `json:"id"`
            Name string `json:"name"`
            Ar []struct {
                Name string `json:"name"`
            } `json:"ar"`
        } `json:"songs"`
        SongCount int `json:"songCount"`
    } `json:"result"`
}

func SearchSong(client *api.Client, title, artist string) (int64, error) {
    url := "https://music.163.com/weapi/cloudsearch/get/web"
    
    // search type: 1 for song
    req := map[string]interface{}{
        "s":      title + " " + artist,
        "type":   1,
        "offset": 0,
        "limit":  3,
        "total":  true,
    }
    
    var reply SearchResp
    opts := api.NewOptions()
    opts.CryptoMode = api.CryptoModeWEAPI
    
    _, err := client.Request(context.Background(), url, req, &reply, opts)
    if err != nil {
        return 0, err
    }
    
    if len(reply.Result.Songs) > 0 {
        // Simple heuristic: return first match. 
        // Could be improved by checking Artist name match.
        return reply.Result.Songs[0].Id, nil
    }
    
    return 0, nil
}

func AddSongToPlaylist(wapi *weapi.Api, playlistID, songID int64) error {
    // Add song to playlist
    // Op: "add"
    req := &weapi.PlaylistAddOrDelReq{
        Op: "add",
        Pid: playlistID,
        TrackIds: types.IntsString{songID},
    }
    
    resp, err := wapi.PlaylistAddOrDel(context.Background(), req)
    if err != nil {
        return err
    }
    
    // API might return 200 even if some songs failed, checking body requires scanning more fields.
    // Assuming if error is nil, it's mostly ok.
    // However, the library doc says: "502 歌单歌曲重复", "404 歌单不存在" might come as error if the library checks status code.
    // If API returns 200 with logical error, we might need to inspect resp.
    // But for now, basic error handling is enough.
    
    // Basic success check
    if resp.Code != 200 {
         return fmt.Errorf("api code: %d", resp.Code)
    }

    return nil
}


func ParseDouyinPlaylist(url string) ([]Song, error) {
	// 1. Get the final URL (resolve redirects)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // Follow redirects
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	html := string(bodyBytes)

	// 2. Extract JSON data using regex
	// Pattern to find `_ROUTER_DATA = {...}`
    // The JSON might be inside a script tag, we look for the specific assignment variable
	// Pattern to find `_ROUTER_DATA = {...}`
    // The JSON might be inside a script tag, we look for the specific assignment variable
	// re := regexp.MustCompile(`_ROUTER_DATA\s*=\s*(\{.*?\})\s*;`) - UNUSED
    // Note: The dot (.) in Go regex does not match newlines by default. 
    // If the JSON spans multiple lines, we might need `(?s)`. 
    // However, looking at the previous file view, it seems to be on a single line or we can capture enough.
    // Let's try a safer regex that captures until the end of the script or semicolon.
    
    // Better regex: `_ROUTER_DATA\s*=\s*(\{.+?\})(?:;|<)`
    // Or just look for the prefix and extract until we find a valid JSON end? 
    // Let's stick to the one shown in HTML snippet: `<script async data-script-src="modern-inline">_ROUTER_DATA = {...}</script>`
    
    // Attempt to just match the start and extract manually if regex is too complex for nested braces.
    startPrefix := "_ROUTER_DATA ="
    startIndex := strings.Index(html, startPrefix)
    if startIndex == -1 {
        return nil, fmt.Errorf("_ROUTER_DATA not found in HTML")
    }
    
    // Find the start of the JSON
    jsonStart := startIndex + len(startPrefix)
    // The JSON seems to be the rest of the script content. 
    remaining := html[jsonStart:]
    
    // Find the first '{' after the assignment
    lbrace := strings.Index(remaining, "{")
    if lbrace == -1 {
         return nil, fmt.Errorf("could not find JSON start '{'")
    }
    
    jsonContent := remaining[lbrace:]
    
	// 3. Parse JSON using Decoder to ignore trailing garbage
	var data RouterData
    decoder := json.NewDecoder(strings.NewReader(jsonContent))
	err = decoder.Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	// 4. Extract songs
	var songs []Song
	medias := data.LoaderData.PlaylistPage.Medias
	for _, media := range medias {
        if media.Type != "track" {
            continue
        }
		trackName := media.Entity.Track.Name
		var artists []string
		for _, artist := range media.Entity.Track.Artists {
			artists = append(artists, artist.Name)
		}
		songs = append(songs, Song{
			Title:  trackName,
			Artist: strings.Join(artists, ", "),
		})
	}

	return songs, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
