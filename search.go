package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// BM25 constants
const k1 float64 = 1.2
const b float64 = 0.75

const maxResults = 10

var (
	docmap  map[string]int64
	lexicon map[string]LexiconEntry
	meta    Metadata
)

type Result struct {
	Id        string  `json:id`
	Relevance float64 `json:relevance`
}

type ResultSlice []*Result

// Len is part of sort.Interface.
func (r ResultSlice) Len() int {
	return len(r)
}

// Swap is part of sort.Interface.
func (r ResultSlice) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

// Less is part of sort.Interface.
func (r ResultSlice) Less(i, j int) bool {
	return r[i].Relevance < r[j].Relevance
}

type QueryResults struct {
	ProcessingTime string      `json:"processing_time"`
	Results        ResultSlice `json:"documents"`
	TotalResults   int64       `json:"total_results"`
}

func loadLexicon() {
	f := openFile(LEXICON_FILE)
	defer f.Close()

	decoder := json.NewDecoder(f)

	if err := decoder.Decode(&lexicon); err != nil {
		fmt.Println("Error loading lexicon")
		panic(err)
	}

	return
}

func loadDocMap() {
	f := openFile(DOC_MAP_FILE)
	defer f.Close()

	decoder := json.NewDecoder(f)

	if err := decoder.Decode(&docmap); err != nil {
		fmt.Println("Error loading docmap")
		panic(err)
	}

	return
}

func loadMetadata() {
	f := openFile(METADATA_FILE)
	defer f.Close()

	s := bufio.NewScanner(f)
	s.Scan()
	line := s.Text()
	err := json.Unmarshal([]byte(line), &meta)

	if err != nil {
		fmt.Println("Error loading metadata")
		panic(err)
	}
}

func bm25(term_freq, doc_freq, doc_length, avg_doc_length, collection_size float64) (score float64) {
	//See: https://en.wikipedia.org/wiki/Okapi_BM25#The_ranking_function

	//term_freq: frequency of term across entire collection
	//doc_freq: frequency of term in a single document
	//doc_length: length of a single document (in words)
	//avg_doc_length: used to prevent longer documents from appearing more
	//relevant, simply because they contain more terms
	N := collection_size

	K := k1 * ((1 - b) + b*(doc_length/avg_doc_length))

	inverse_doc_freq := math.Log(((N - term_freq + 0.5) / (term_freq + 0.5)))

	return inverse_doc_freq * ((k1 + 1) * doc_freq) / (K + doc_freq)
}
func postingLookup(term string, filePos int64) (entries []IDF) {
	f := openFile(POSTING_FILE)
	defer f.Close()

	// 0 means relative to beginning of file
	f.Seek(filePos, 0)

	s := bufio.NewScanner(f)
	s.Scan()
	line := s.Text()

	err := json.Unmarshal([]byte(line), &entries)
	if err != nil {
		fmt.Println("Error reading line from posting file")
		panic(err)
	}

	return
}

func processQuery(terms []string) (docScores map[string]float64) {
	loadMetadata()
	loadLexicon()
	loadDocMap()

	docScores = make(map[string]float64)

	isStopWord := getStopFunc()

	for _, term := range terms {
		term = strings.ToLower(term)
		if isStopWord(term) {
			continue
		}

		lexEntry, present := lexicon[term]
		if !present {
			// term is not in our corpus :(
			continue
		}

		entries := postingLookup(term, lexEntry.FilePos)

		for _, postEntry := range entries {
			docLen, _ := docmap[postEntry.DocId]
			score := bm25(
				float64(lexEntry.Frequency),
				float64(postEntry.TF),
				float64(docLen),
				float64(meta.AverageLength),
				float64(meta.CorpusSize),
			)
			docScores[postEntry.DocId] = score
		}
	}
	// need to sort scores in descending order, and truncate if > max results

	return docScores
}

func search(terms []string) (qr *QueryResults) {
	start := timestamp()

	relevanceScores := processQuery(terms)
	results := make(ResultSlice, 0)

	for k, v := range relevanceScores {
		results = append(results, &Result{Id: k, Relevance: v})
	}

	sort.Sort(sort.Reverse(results))
	totalResults := int64(len(results))
	if totalResults > maxResults {
		// some crude pagination
		results = results[:maxResults]
	}

	processTime := fmt.Sprintf("%s", time.Since(start))
	qr = &QueryResults{
		Results:        results,
		ProcessingTime: processTime,
		TotalResults:   totalResults,
	}

	return
}

func cmdSearch(terms []string) {
	// used to print out query results when called from commandline
	results := search(terms)

	fmt.Println("Doc Id | Relevance")
	fmt.Println("------------------")
	for _, result := range results.Results {
		fmt.Println(result.Id, ":", result.Relevance)
	}

	fmt.Println("Returning", len(results.Results), "of", results.TotalResults, "documents that match your query:", query)
	fmt.Printf("Query processed in %s\n", results.ProcessingTime)
}
