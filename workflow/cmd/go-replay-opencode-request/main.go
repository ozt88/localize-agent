package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type result struct {
	Timestamp     string `json:"timestamp"`
	URL           string `json:"url"`
	BodyBytes     int    `json:"body_bytes"`
	OK            bool   `json:"ok"`
	Status        int    `json:"status,omitempty"`
	ResponseBytes int    `json:"response_bytes,omitempty"`
	ResponseRaw   string `json:"response_raw,omitempty"`
	Error         string `json:"error,omitempty"`
}

func main() {
	serverURL := flag.String("server-url", "", "base server url, e.g. http://127.0.0.1:4112")
	path := flag.String("path", "", "request path, e.g. /session/xxx/message")
	bodyFile := flag.String("body-file", "", "utf-8 body file")
	bodyBase64File := flag.String("body-base64-file", "", "base64-encoded body file")
	directory := flag.String("directory", "", "directory query value")
	outFile := flag.String("out-file", "", "optional json result output path")
	timeoutSec := flag.Int("timeout-sec", 120, "http timeout seconds")
	flag.Parse()

	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*path) == "" {
		fail("server-url and path are required")
	}
	if strings.TrimSpace(*bodyFile) == "" && strings.TrimSpace(*bodyBase64File) == "" {
		fail("one of body-file or body-base64-file is required")
	}

	bodyBytes, err := loadBody(*bodyFile, *bodyBase64File)
	if err != nil {
		fail(err.Error())
	}

	fullURL, err := buildURL(*serverURL, *path, *directory)
	if err != nil {
		fail(err.Error())
	}

	res := result{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		URL:       fullURL,
		BodyBytes: len(bodyBytes),
	}

	client := &http.Client{Timeout: time.Duration(*timeoutSec) * time.Second}
	req, err := http.NewRequest(http.MethodPost, fullURL, bytes.NewReader(bodyBytes))
	if err != nil {
		res.Error = err.Error()
		writeResult(res, *outFile)
		return
	}
	req.Header.Set("content-type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		writeResult(res, *outFile)
		return
	}
	defer resp.Body.Close()

	payload, readErr := io.ReadAll(resp.Body)
	res.Status = resp.StatusCode
	res.ResponseBytes = len(payload)
	res.ResponseRaw = string(payload)
	if readErr != nil {
		res.Error = fmt.Sprintf("read response body: %v", readErr)
		writeResult(res, *outFile)
		return
	}
	res.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !res.OK {
		res.Error = fmt.Sprintf("http %d", resp.StatusCode)
	}

	writeResult(res, *outFile)
}

func loadBody(bodyFile, bodyBase64File string) ([]byte, error) {
	if strings.TrimSpace(bodyBase64File) != "" {
		raw, err := os.ReadFile(bodyBase64File)
		if err != nil {
			return nil, err
		}
		text := strings.TrimSpace(string(raw))
		text = strings.TrimPrefix(text, "\ufeff")
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return os.ReadFile(bodyFile)
}

func buildURL(serverURL, path, directory string) (string, error) {
	u, err := url.Parse(strings.TrimRight(serverURL, "/") + path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(directory) != "" {
		q := u.Query()
		q.Set("directory", directory)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func writeResult(res result, outFile string) {
	b, _ := json.MarshalIndent(res, "", "  ")
	if strings.TrimSpace(outFile) != "" {
		_ = os.WriteFile(outFile, b, 0o644)
	}
	fmt.Println(string(b))
}

func fail(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}
