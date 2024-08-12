package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
)

// menghubungkan ke model AI.
type AIModelConnector struct {
	Client *http.Client
	Token  string
}

// mengirimkan data ke model AI.
type Payload struct {
	Inputs string `json:"inputs"`
}

// untuk tabel dan query yang dikirim ke model AI.
type Inputs struct {
	Table map[string][]string `json:"table"`
	Query string              `json:"query"`
}

// data yang dikembalikan model AI.
type Response struct {
	Answer      string   `json:"answer"`
	Coordinates [][]int  `json:"coordinates"`
	Cells       []string `json:"cells"`
	Aggregator  string   `json:"aggregator"`
}

// menerima string dari file CSV sebagai input dan mengembalikan `map`
// `key`-nya header kolom dan `value`nya data untuk setiap kolom.
func (c *AIModelConnector) CsvToSlice(data string) (map[string][]string, error) {
	reader := csv.NewReader(strings.NewReader(data))

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	result := make(map[string][]string)

	headers := records[0]

	for _, header := range headers {
		result[header] = []string{}
	}

	for _, record := range records[1:] {
		for i, value := range record {
			header := headers[i]
			result[header] = append(result[header], value)
		}
	}

	return result, nil
}

// menghubungkan ke model AI dan mengirimkan respons.
func (c *AIModelConnector) ConnectAIModel(payload interface{}, token string) (Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequest("POST", "https://api-inference.huggingface.co/models/google/tapas-base-finetuned-wtq", bytes.NewBuffer(jsonData))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("received non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Response{}, err
	}

	return response, nil
}

// menghubungkan ke model GPT2 dan mengirimkan respons.
func (c *AIModelConnector) callGPT2Model(inputText string) (string, error) {
	url := "https://api-inference.huggingface.co/models/gpt2"
	token := os.Getenv("HUGGINGFACE_TOKEN")

	payload := Payload{
		Inputs: inputText,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// Fungsi baru untuk menerjemahkan teks
func translateText(text string) (string, error) {
	url := "https://api-inference.huggingface.co/models/Helsinki-NLP/opus-mt-id-en"
	payload := map[string]string{"inputs": text}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer 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")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Print the raw response for debugging
	fmt.Printf("Raw translation response: %s\n", string(body))

	var result []struct {
		TranslationText string `json:"translation_text"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if len(result) > 0 {
		return result[0].TranslationText, nil
	}

	return "", fmt.Errorf("no translation found in response")
}

// menyajikan file `index.html`.
func (c *AIModelConnector) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// memproses CSV dan query dari permintaan, mengirimkannya ke model AI, dan
// mengembalikan respons.
func (c *AIModelConnector) handleJawab(w http.ResponseWriter, r *http.Request) {
	req := &struct {
		CSV string `json:"csv"`
		Ask string `json:"ask"`
	}{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		fmt.Println(err)
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{
			Success: false,
		})
		return
	}
	sliceData, err := c.CsvToSlice(req.CSV)
	if err != nil {
		fmt.Println(err)
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{
			Success: false,
		})
		return
	}
	translatedQuery, err := translateText(req.Ask)
	if err != nil {
		fmt.Printf("Error translating query: %v\n", err)
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{
			Success: false,
		})
		return
	}

	payload := Inputs{
		Table: sliceData,
		Query: translatedQuery,
	}

	response, err := c.ConnectAIModel(payload, c.Token)
	if err != nil {
		fmt.Printf("Error from AI model: %v\n", err)
		fmt.Println(err)
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{
			Success: false,
		})
		return
	}

	total := 0.0
	ansTmp := ""
	if response.Aggregator == "SUM" {
		for _, cell := range response.Cells {
			value, err := strconv.ParseFloat(strings.TrimSpace(cell), 64)
			if err != nil {
				err_msg := fmt.Sprintf("Error converting cell to float: %v", err)
				fmt.Println(err_msg)
			}
			total += value
		}
		ansTmp = fmt.Sprintf("%f", total)
	} else {
		ansTmp = fmt.Sprintf("AI Model Response: \n%+v", response.Cells)
	}

	//response2, err := c.callGPT2Model("what is your suggestion to " + ansTmp)
	if err != nil {
		fmt.Println(err)
		json.NewEncoder(w).Encode(struct {
			Success bool `json:"success"`
		}{
			Success: false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Success     bool     `json:"success"`
		Answer      string   `json:"answer"`
		Coordinates [][]int  `json:"coordinates"`
		Cells       []string `json:"cells"`
		Aggregator  string   `json:"aggregator"`
	}{
		Success:     true,
		Answer:      ansTmp,
		Coordinates: response.Coordinates,
		Cells:       response.Cells,
		Aggregator:  response.Aggregator,
	})
}

// Mengatur server HTTP dan rute.
func main() {
	token := "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"
	if token == "" {
		fmt.Println("Error: HUGGINGFACE_TOKEN is not set")
		return
	}
	c := AIModelConnector{
		Client: &http.Client{},
		Token:  token,
	}
	http.HandleFunc("/", c.handleIndex)
	http.HandleFunc("/jawab", c.handleJawab)
	fmt.Println("Server berjalan di port 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Gagal memulai server:", err)
		os.Exit(1)
	}
}
