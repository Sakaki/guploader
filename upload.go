package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/net/context"
	"encoding/json"
	"io"
	"path"
	"path/filepath"
	"time"
	"net/http"
	"strings"
	"regexp"
	"flag"
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
	var err error
	currentDir := path.Dir(os.Args[0])
	// オプション読み込み
	authOnly := flag.Bool("authOnly", false, "Only execute authorization.")
	noLoop := flag.Bool("noLoop", false, "Upload photos once.")
	flag.Parse()
	// 設定ファイル読み込み
	settingsPath := fmt.Sprintf("%s/%s", currentDir, "settings.json")
	settingsJsonRaw, err := ioutil.ReadFile(settingsPath)
	if err != nil {
		log.Fatalf("設定ファイルを読み込めませんでした: %s", err)
	}
	settings := new(Settings)
	if err := json.Unmarshal(settingsJsonRaw, settings); err != nil {
		log.Fatalf("設定ファイルを正しくパース出来ませんでした: %s", err)
	}
	client := getApiClient(currentDir, settings)
	if *authOnly {
		return
	}
	var photoList string
	for photoList == "" {
		photoList, err = getPhotoList(settings.UserID, settings.AlbumID, client)
		if err != nil {
			fmt.Println("Connection failed. Wait and retry...")
			time.Sleep(5 * time.Second)
		}
	}
	log.Println(photoList)
	log.Println(len(photoList))
	if *noLoop {
		photoList = execUpload(settings, client, photoList)
	} else {
		for ;; {
			photoList = execUpload(settings, client, photoList)
			time.Sleep(1 * time.Second)
		}
	}
}

func getApiClient(currentDir string, settings *Settings) *http.Client {
	// 認証設定
	authConf := oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{"https://picasaweb.google.com/data/"},
	}
	tokenStorePath := fmt.Sprintf("%s/%s", currentDir, "token_cached.json")
	var token *oauth2.Token
	if Exists(tokenStorePath) {
		tokenJsonRaw, _ := ioutil.ReadFile(tokenStorePath)
		token = new(oauth2.Token)
		if err := json.Unmarshal(tokenJsonRaw, token); err != nil {
			log.Fatalf("Error while loading saved token: %s", err)
		}
	} else {
		// 認証のURLを取得。AuthCodeURLには文字列を渡す
		url := authConf.AuthCodeURL("test")
		fmt.Println(url)
		// リダイレクト先がないため、ブラウザで認証後に表示されるコードを入力
		var s string
		var sc = bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			s = sc.Text()
		}
		// アクセストークンを取得
		var err error
		token, err = authConf.Exchange(context.Background(), s)
		if err != nil {
			log.Fatalf("exchange error: %s", err)
		}
		// トークンを保存
		tokenBytes, err := json.Marshal(token)
		if err == nil {
			ioutil.WriteFile(tokenStorePath, tokenBytes, os.ModePerm)
		}
	}
	// httpクライアントを取得
	client := authConf.Client(context.Background(), token)
	return client
}

func getPhotoList(userId string, albumId string, client *http.Client) (string, error) {
	url := fmt.Sprintf("https://picasaweb.google.com/data/feed/api/user/%s/albumid/%s", userId, albumId)
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error while getting album %s", err)
		return "", err
	}
	defer resp.Body.Close()
	// データの読み込み
	respBuf := make([]byte, 256)
	totalSize := 0
	albumData := ""
	prevData := ""
	// 画像ファイル名のみ取得する
	photoNamePattern := regexp.MustCompile(`<media:title type='plain'>(.*)</media:title>`)
	// 一気に読むとRAMが足りないので少しずつ読む
	for {
		n, err := resp.Body.Read(respBuf)
		totalSize += n
		// 読み込み終了かエラーだったら終了
		if n == 0 || err == io.EOF {
			break
		} else if err != nil {
			fmt.Println("Read response body error:", err)
			break
		}
		currentData := string(respBuf[:n])
		// もし画像ファイルが存在したらリスティングする
		for index, photoName := range photoNamePattern.FindStringSubmatch(prevData + currentData) {
			if index != 0 && !strings.Contains(albumData, photoName) {
				albumData += photoName
			}
		}
		// ファイル名が途切れていた場合、次のループで検索
		prevData = currentData
	}
	log.Printf("Total: %d\n", totalSize)
	return albumData, nil
}

func execUpload(settings *Settings, client *http.Client, photoList string) string {
	// 対象ディレクトリ内のファイルを送信
	files, err := ioutil.ReadDir(settings.TargetDir)
	if err != nil {
		log.Fatalf("Error loading directory: %s", err)
	}
	for _, file := range files {
		targetPath := filepath.Join(settings.TargetDir, file.Name())
		dateFormat := file.ModTime().Format("2006_01_02_15_04_05")
		extension := path.Ext(file.Name())
		filenameWithDate := file.Name()[0:len(file.Name()) - len(extension)] + "_" + dateFormat
			log.Println(filenameWithDate)
		// ファイルが送信済みかjpgでない場合はスキップ
		if path.Ext(targetPath) != ".JPG" || strings.Contains(photoList, filenameWithDate) {
			log.Printf("Skipping %s", targetPath)
			continue
		}
		// POST
		file, err := os.Open(targetPath)
		if err != nil {
			log.Fatalf("Error loading %s: %s", targetPath, err)
		}
		buf := io.Reader(file)
		urlFormat := "https://picasaweb.google.com/data/feed/api/user/%s/albumid/%s"
		url := fmt.Sprintf(urlFormat, settings.UserID, settings.AlbumID)
		req, _ := http.NewRequest("POST", url, buf)
		req.Header.Set("Content-Type", "image/jpeg")
		req.Header.Set("Slug", filenameWithDate)
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
		photoList += filenameWithDate
		log.Println(photoList)
	}
	return photoList
}
