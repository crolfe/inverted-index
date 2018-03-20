package main

type Accumulator struct {
	Values map[string]int64
}

func NewAccumulator() *Accumulator {
	return &Accumulator{make(map[string]int64)}
}

func (a *Accumulator) Add(s string, amount int64) {
	count, present := a.Values[s]
	if !present {
		count = amount
	} else {
		count += amount
	}

	a.Values[s] = count
}

func (a *Accumulator) GetValue(term string) (value int64) {
	value, _ = a.Values[term]

	return value
}

type Lexicon struct {
	Term  string `json:"term"`
	Entry LexiconEntry
}

type LexiconEntry struct {
	Frequency int64 `json:"frequency"`
	FilePos   int64 `json:"file_pos"`
}

func NewLexicon(term string, freq, filePos int64) *Lexicon {
	entry := LexiconEntry{FilePos: filePos, Frequency: freq}
	return &Lexicon{Term: term, Entry: entry}
}

type PostingEntry struct {
	Words Accumulator
	DocId string
}

type IDF struct {
	DocId string
	TF    int64
}

type DocMap struct {
	DocId  string `json:"id"`
	Length int64  `json:"length"`
}

type ByDocId []IDF

type Channels struct {
	docmap      chan DocMap
	lexicon     chan Lexicon
	posting     chan PostingEntry
	parsingDone chan struct{}
	postingDone chan struct{}
	docLength   chan int
	corpusSize  chan int
}

func NewChannels() *Channels {
	return &Channels{
		docmap:      make(chan DocMap, 250),
		lexicon:     make(chan Lexicon, 250),
		posting:     make(chan PostingEntry, 250),
		parsingDone: make(chan struct{}),
		postingDone: make(chan struct{}),
		docLength:   make(chan int, 250),
		corpusSize:  make(chan int),
	}
}

type Metadata struct {
	AverageLength int `json:"average_length"`
	CorpusSize    int `json:"corpus_size"`
}

type Document struct {
	Id         string   `xml:"DOCID"`
	Number     string   `xml:"DOCNO"`
	Date       string   `xml:"DATE>P"`
	Length     string   `xml:"LENGTH>P"`
	Headline   []string `xml:"HEADLINE>P"`
	Byline     string   `xml:"BYLINE>P"`
	Paragraphs []string `xml:"TEXT>P"`
	Graphic    []string `xml:"GRAPHIC>P"`
}

type Corpus struct {
	Documents []Document `xml:"DOC"`
}

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
