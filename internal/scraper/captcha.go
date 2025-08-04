package scraper

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type CaptchaHandler struct {
	MaxRetries      int
	Timeout         time.Duration
	TwoCaptchaKey   string
	AntiCaptchaKey  string
}

type TwoCaptchaResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
}

func (s *Scraper) handleCaptcha(page *rod.Page) error {
	captchaImg, err := page.Element("img#captcha_image, img[id*='captcha'], img[src*='captcha']")
	if err != nil || captchaImg == nil {
		s.logger.Debug("No CAPTCHA detected")
		return nil
	}

	s.logger.Info("CAPTCHA detected, attempting to solve")

	captchaData, err := s.getCaptchaImage(page, captchaImg)
	if err != nil {
		return fmt.Errorf("failed to get CAPTCHA image: %w", err)
	}

	var captchaText string
	var solveErr error

	twoCaptchaKey := os.Getenv("TWOCAPTCHA_API_KEY")
	if twoCaptchaKey != "" {
		s.logger.Debug("Attempting to solve CAPTCHA with 2Captcha")
		captchaText, solveErr = s.solveCaptchaWith2Captcha(captchaData, twoCaptchaKey)
		if solveErr == nil && captchaText != "" {
			s.logger.Info("CAPTCHA solved successfully with 2Captcha")
		} else {
			s.logger.Warn("2Captcha failed", "error", solveErr)
		}
	}

	if captchaText == "" {
		antiCaptchaKey := os.Getenv("ANTICAPTCHA_API_KEY")
		if antiCaptchaKey != "" {
			s.logger.Debug("Attempting to solve CAPTCHA with Anti-Captcha")
			captchaText, solveErr = s.solveCaptchaWithAntiCaptcha(captchaData, antiCaptchaKey)
			if solveErr == nil && captchaText != "" {
				s.logger.Info("CAPTCHA solved successfully with Anti-Captcha")
			} else {
				s.logger.Warn("Anti-Captcha failed", "error", solveErr)
			}
		}
	}

	if captchaText == "" && !s.cfg.HeadlessMode {
		s.logger.Info("Waiting for manual CAPTCHA input")
		captchaText, solveErr = s.waitForManualCaptchaInput(page)
		if solveErr != nil {
			return fmt.Errorf("manual CAPTCHA input failed: %w", solveErr)
		}
	}

	if captchaText == "" {
		captchaID := fmt.Sprintf("captcha_%d", time.Now().Unix())
		if err := s.saveCaptchaForManualSolving(captchaID, captchaData); err == nil {
			s.logger.Info("CAPTCHA saved for manual solving", "id", captchaID)
			captchaText, solveErr = s.waitForManualSolution(captchaID, 60*time.Second)
		}
	}

	if captchaText == "" {
		return fmt.Errorf("failed to solve CAPTCHA with all available methods")
	}

	captchaInput, err := page.Element("input[name='captcha'], input[id*='captcha'], input[type='text'][placeholder*='captcha']")
	if err != nil {
		return fmt.Errorf("CAPTCHA input field not found")
	}

	captchaInput.MustInput(captchaText)
	s.logger.Debug("CAPTCHA text entered", "length", len(captchaText))

	return nil
}

func (s *Scraper) getCaptchaImage(page *rod.Page, captchaImg *rod.Element) ([]byte, error) {
	src, err := captchaImg.Attribute("src")
	if err == nil && src != nil && *src != "" {
		if strings.HasPrefix(*src, "data:image") {
			parts := strings.Split(*src, ",")
			if len(parts) == 2 {
				data, err := base64.StdEncoding.DecodeString(parts[1])
				if err == nil {
					return data, nil
				}
			}
		}

		if strings.HasPrefix(*src, "http") || strings.HasPrefix(*src, "/") {
			imgURL := *src
			if strings.HasPrefix(imgURL, "/") {
				pageURL := page.MustInfo().URL
				base := strings.Split(pageURL, "/")[:3]
				imgURL = strings.Join(base, "/") + imgURL
			}

			cookies := page.MustCookies()
			return s.fetchImageWithCookies(imgURL, cookies)
		}
	}

	screenshot, err := captchaImg.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to screenshot CAPTCHA: %w", err)
	}

	return screenshot, nil
}

func (s *Scraper) fetchImageWithCookies(imgURL string, cookies []*proto.NetworkCookie) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return nil, err
	}

	for _, cookie := range cookies {
		req.AddCookie(&http.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		})
	}

	req.Header.Set("User-Agent", s.cfg.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (s *Scraper) solveCaptchaWith2Captcha(imageData []byte, apiKey string) (string, error) {
	submitURL := "http://2captcha.com/in.php"
	
	formData := url.Values{
		"key":    {apiKey},
		"method": {"base64"},
		"body":   {base64.StdEncoding.EncodeToString(imageData)},
		"json":   {"1"},
	}

	resp, err := http.PostForm(submitURL, formData)
	if err != nil {
		return "", fmt.Errorf("failed to submit to 2captcha: %w", err)
	}
	defer resp.Body.Close()

	var submitResp TwoCaptchaResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return "", fmt.Errorf("failed to decode 2captcha response: %w", err)
	}

	if submitResp.Status != 1 {
		return "", fmt.Errorf("2captcha submission failed: %s", submitResp.Request)
	}

	captchaID := submitResp.Request

	resultURL := fmt.Sprintf("http://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", apiKey, captchaID)
	
	for i := 0; i < 30; i++ {
		time.Sleep(3 * time.Second)

		resp, err := http.Get(resultURL)
		if err != nil {
			continue
		}

		var resultResp TwoCaptchaResponse
		json.NewDecoder(resp.Body).Decode(&resultResp)
		resp.Body.Close()

		if resultResp.Status == 1 {
			return resultResp.Request, nil
		}

		if resultResp.Request != "CAPCHA_NOT_READY" {
			return "", fmt.Errorf("2captcha error: %s", resultResp.Request)
		}
	}

	return "", fmt.Errorf("2captcha timeout")
}

func (s *Scraper) solveCaptchaWithAntiCaptcha(imageData []byte, apiKey string) (string, error) {
	
	type CreateTaskRequest struct {
		ClientKey string `json:"clientKey"`
		Task      struct {
			Type string `json:"type"`
			Body string `json:"body"`
		} `json:"task"`
	}

	type TaskResultResponse struct {
		ErrorId          int    `json:"errorId"`
		Status           string `json:"status"`
		Solution         struct {
			Text string `json:"text"`
		} `json:"solution"`
	}

	createReq := CreateTaskRequest{ClientKey: apiKey}
	createReq.Task.Type = "ImageToTextTask"
	createReq.Task.Body = base64.StdEncoding.EncodeToString(imageData)

	jsonData, _ := json.Marshal(createReq)
	
	resp, err := http.Post(
		"https://api.anti-captcha.com/createTask",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var createResp struct {
		ErrorId int `json:"errorId"`
		TaskId  int `json:"taskId"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", err
	}

	if createResp.ErrorId != 0 {
		return "", fmt.Errorf("anti-captcha error: %d", createResp.ErrorId)
	}

	for i := 0; i < 30; i++ {
		time.Sleep(3 * time.Second)

		getResultReq := map[string]interface{}{
			"clientKey": apiKey,
			"taskId":    createResp.TaskId,
		}
		
		jsonData, _ := json.Marshal(getResultReq)
		resp, err := http.Post(
			"https://api.anti-captcha.com/getTaskResult",
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			continue
		}

		var result TaskResultResponse
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result.Status == "ready" {
			return result.Solution.Text, nil
		}
	}

	return "", fmt.Errorf("anti-captcha timeout")
}

func (s *Scraper) waitForManualCaptchaInput(page *rod.Page) (string, error) {
	// Show alert to user
	page.MustEval(`() => alert("Please enter the CAPTCHA and click OK")`)
	
	captchaInput, err := page.Element("input[name='captcha'], input[id*='captcha']")
	if err != nil {
		return "", err
	}

	for i := 0; i < 60; i++ {
		time.Sleep(1 * time.Second)
		
		value, err := captchaInput.Property("value")
		if err == nil && value.String() != "" {
			return value.String(), nil
		}
	}

	return "", fmt.Errorf("manual CAPTCHA input timeout")
}

func (s *Scraper) saveCaptchaForManualSolving(captchaID string, imageData []byte) error {
	captchaDir := "./data/captchas"
	os.MkdirAll(captchaDir, 0755)
	
	filename := fmt.Sprintf("%s/%s.png", captchaDir, captchaID)
	return os.WriteFile(filename, imageData, 0644)
}

func (s *Scraper) waitForManualSolution(captchaID string, timeout time.Duration) (string, error) {
	solutionFile := fmt.Sprintf("./data/captchas/%s.txt", captchaID)
	
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(solutionFile); err == nil {
			solution := strings.TrimSpace(string(data))
			if solution != "" {
				os.Remove(solutionFile)
				os.Remove(fmt.Sprintf("./data/captchas/%s.png", captchaID))
				return solution, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	
	return "", fmt.Errorf("timeout waiting for manual solution")
}

func (s *Scraper) verifyCaptchaSuccess(page *rod.Page) bool {
	errorMsg, err := page.Element(".captcha-error, .error-message, span.error")
	if err == nil && errorMsg != nil {
		text, _ := errorMsg.Text()
		if strings.Contains(strings.ToLower(text), "captcha") || 
		   strings.Contains(strings.ToLower(text), "invalid") {
			s.logger.Warn("CAPTCHA error detected", "error", text)
			return false
		}
	}

	return true
}

func (s *Scraper) retryCaptcha(page *rod.Page, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		s.logger.Info("Retrying CAPTCHA", "attempt", i+1)

		refreshBtn, err := page.Element("img[onclick*='captcha'], a[onclick*='captcha'], button[onclick*='captcha']")
		if err == nil && refreshBtn != nil {
			refreshBtn.MustClick()
			time.Sleep(1 * time.Second)
		}

		if err := s.handleCaptcha(page); err != nil {
			s.logger.Warn("CAPTCHA retry failed", "attempt", i+1, "error", err)
			continue
		}

		if s.verifyCaptchaSuccess(page) {
			return nil
		}
	}

	return fmt.Errorf("failed to solve CAPTCHA after %d attempts", maxRetries)
}