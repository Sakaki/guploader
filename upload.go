package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"encoding/json"
	"io"
	"path"
	"path/filepath"
	"time"
	"net/http"
	"strings"
	"regexp"
)

type Settings struct {
	UserID string `json:"user_id"`
	AlbumID string `json:"album_id"`
	ClientID string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	TargetDir string `json:"target_dir"`
}

func Exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func main() {
	current_dir := path.Dir(os.Args[0])
	// 設定ファイル読み込み
	settings_path := fmt.Sprintf("%s/%s", current_dir, "settings.json")
	settings_json_raw, _ := ioutil.ReadFile(settings_path)
	settings := new(Settings)
	if err := json.Unmarshal(settings_json_raw, settings); err != nil {
		log.Fatalf("JSONを正しくパース出来ませんでした: %s", err)
	}
	client := get_api_client(current_dir, settings)

	var err error
	var photo_list string
	for photo_list == "" {
		photo_list, err = get_photo_list(settings.UserID, settings.AlbumID, client)
		if err != nil {
			fmt.Println("Connection failed. Wait and retry...")
			time.Sleep(5 * time.Second)
		}
	}
	log.Println(photo_list)
	log.Println(len(photo_list))
	for ;; {
		photo_list = exec_upload(settings, client, photo_list)
		time.Sleep(1 * time.Second)
	}
}

func get_api_client(current_dir string, settings *Settings) *http.Client {
	// 認証設定
	auth_conf := oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{"https://picasaweb.google.com/data/"},
	}
	token_store_path := fmt.Sprintf("%s/%s", current_dir, "token_cached.json")
	var token *oauth2.Token
	if Exists(token_store_path) {
		token_json_raw, _ := ioutil.ReadFile(token_store_path)
		token = new(oauth2.Token)
		if err := json.Unmarshal(token_json_raw, token); err != nil {
			log.Fatalf("Error while loading saved token: %s", err)
		}
	} else {
		// 認証のURLを取得。AuthCodeURLには文字列を渡す
		url := auth_conf.AuthCodeURL("test")
		fmt.Println(url)
		// リダイレクト先がないため、ブラウザで認証後に表示されるコードを入力
		var s string
		var sc = bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			s = sc.Text()
		}
		// アクセストークンを取得
		var err error
		token, err = auth_conf.Exchange(oauth2.NoContext, s)
		if err != nil {
			log.Fatalf("exchange error: %s", err)
		}
	}
	// httpクライアントを取得
	client := auth_conf.Client(oauth2.NoContext, token)
	return client
}

func get_photo_list(user_id string, album_id string, client *http.Client) (string, error) {
	url := fmt.Sprintf("https://picasaweb.google.com/data/feed/api/user/%s/albumid/%s", user_id, album_id)
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error while getting album %s", err)
		return "", err
	}
	defer resp.Body.Close()
	// データの読み込み
	resp_buf := make([]byte, 256)
	total_size := 0
	album_data := ""
	prev_data := ""
	// 画像ファイル名のみ取得する
	photo_name_pattern := regexp.MustCompile(`<media:title type='plain'>(.*)</media:title>`)
	// 一気に読むとRAMが足りないので少しずつ読む
	for {
		n, err := resp.Body.Read(resp_buf)
		total_size += n
		// 読み込み終了かエラーだったら終了
		if n == 0 || err == io.EOF {
			break
		} else if err != nil {
			fmt.Println("Read response body error:", err)
			break
		}
		current_data := string(resp_buf[:n])
		// もし画像ファイルが存在したらリスティングする
		for index, photo_name := range photo_name_pattern.FindStringSubmatch(prev_data + current_data) {
			if index != 0 && !strings.Contains(album_data, photo_name) {
				album_data += photo_name
			}
		}
		// ファイル名が途切れていた場合、次のループで検索
		prev_data = current_data
	}
	log.Printf("Total: %d\n", total_size)
	return album_data, nil
}

func exec_upload(settings *Settings, client *http.Client, photo_list string) string {
	// 対象ディレクトリ内のファイルを送信
	files, err := ioutil.ReadDir(settings.TargetDir)
	if err != nil {
		log.Fatalf("Error loading directory: %s", err)
	}
	for _, file := range files {
		target_path := filepath.Join(settings.TargetDir, file.Name())
		date_format := file.ModTime().Format("2006_01_02_15_04_05")
		extension := path.Ext(file.Name())
		filename_with_date := file.Name()[0:len(file.Name()) - len(extension)] + "_" + date_format
			log.Println(filename_with_date)
		// ファイルが送信済みかjpgでない場合はスキップ
		if path.Ext(target_path) != ".JPG" || strings.Contains(photo_list, filename_with_date) {
			log.Printf("Skipping %s", target_path)
			continue
		}
		// POST
		file, err := os.Open(target_path)
		if err != nil {
			log.Fatalf("Error loading %s: %s", target_path, err)
		}
		buf := io.Reader(file)
		url_format := "https://picasaweb.google.com/data/feed/api/user/%s/albumid/%s"
		url := fmt.Sprintf(url_format, settings.UserID, settings.AlbumID)
		req, _ := http.NewRequest("POST", url, buf)
		req.Header.Set("Content-Type", "image/jpeg")
		req.Header.Set("Slug", filename_with_date)
		// resp, err := client.Post(url, "image/jpeg", buf)
		resp, err := client.Do(req)
		// resp, err := client.Get("https://picasaweb.google.com/data/feed/api/user/default?start-index=1")
		if err != nil {
			log.Printf("Error while sending file %s", err)
			continue
		}
		//レスポンスを表示
		fmt.Printf("Image sent with status code: %d\n", resp.StatusCode)
		resp.Body.Close()
		photo_list += filename_with_date
		log.Println(photo_list)
	}
	return photo_list
}
