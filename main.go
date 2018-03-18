package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	corpus, stoplist string
)
var (
	docMapWG, parseWG, postingWG, lexiconWG sync.WaitGroup
)

type Accumulator struct {
	Values map[string]int
}

func NewAccumulator() *Accumulator {
	return &Accumulator{make(map[string]int)}
}

func (a *Accumulator) Add(s string, amount int) {
	count, present := a.Values[s]
	if !present {
		count = amount
	} else {
		count += amount
	}

	a.Values[s] = count
}

func (a *Accumulator) GetValue(term string) (value int) {
	value, _ = a.Values[term]

	return value
}

type LexiconEntry struct {
	Term      string
	Frequency int
	FilePos   int64
}

type PostingEntry struct {
	Words Accumulator
	DocId string
}

type IDF struct {
	DocId string
	TF    int
}

type DocMap struct {
	DocId  string
	Length int
}

type ByDocId []IDF

type Channels struct {
	docmap      chan DocMap
	lexicon     chan LexiconEntry
	posting     chan PostingEntry
	quit        chan struct{}
	postingDone chan struct{}
}

func NewChannels() *Channels {
	return &Channels{
		docmap:      make(chan DocMap, 250),
		lexicon:     make(chan LexiconEntry, 250),
		posting:     make(chan PostingEntry, 250),
		quit:        make(chan struct{}),
		postingDone: make(chan struct{}),
	}
}

//func (a ByAge) Len() int           { return len(a) }
//func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
//func (a ByAge) Less(i, j int) bool { return a[i].DocId < a[j].DocId }

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

func openFile(filename string) (f *os.File) {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file: ", err.Error())
		os.Exit(1)
	}
	return
}

func init() {
	flag.StringVar(&corpus, "corpus", "", "<corpus>")
	flag.StringVar(&stoplist, "stoplist", "", "<stoplist>")
}

func loadStopList(filename string) (stopList map[string]bool) {
	f := openFile(filename)
	defer f.Close()

	s := bufio.NewScanner(f)

	stopList = make(map[string]bool)

	for s.Scan() {
		word := strings.TrimSpace(s.Text())
		// ignore the bool value, as we are using this map like a set
		stopList[word] = true
	}

	return
}

func getStopFunc() func(s string) bool {
	stopList := loadStopList(stoplist)

	return func(s string) (present bool) {
		_, present = stopList[s]
		return
	}
}

func parseDocument(d Document, isStopWord func(w string) bool, c *Channels) {
	defer parseWG.Done()

	d.Number = strings.TrimSpace(d.Number)
	d.Id = strings.TrimSpace(d.Id)
	docFreq := NewAccumulator()

	words := make([]string, 0)

	countWords := func(s string) {
		stripped := strings.Replace(s, "\n", "", -1)
		for _, w := range strings.Split(strings.ToLower(stripped), " ") {
			w := strings.TrimSpace(w)

			if !isStopWord(w) {
				words = append(words, w)
			}
		}
	}

	for _, headline := range d.Headline {
		countWords(headline)
	}
	countWords(d.Byline)

	for _, p := range d.Paragraphs {
		countWords(p)
	}

	sort.Sort(sort.StringSlice(words))
	for _, w := range words {
		docFreq.Add(w, 1)
	}

	doc := PostingEntry{Words: *docFreq, DocId: d.Id}
	c.posting <- doc

	docmap := DocMap{DocId: d.Id, Length: len(words)}
	c.docmap <- docmap

	return
}

func buildLexicon(c *Channels) {
	defer lexiconWG.Done()

	entries := make([]LexiconEntry, 0)

	f, err := os.Create("./lexicon.txt")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	for {
		select {
		case entry := <-c.lexicon:
			entries = append(entries, entry)
		case <-c.postingDone:
			for _, entry := range entries {
				line := fmt.Sprintf("%s %d\n", entry.Term, entry.FilePos)
				f.WriteString(line)
			}
			f.Sync()
			return
		}
	}
}

func buildPosting(c *Channels) {
	defer postingWG.Done()

	f, err := os.Create("./posting.txt")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	IDFs := make(map[string][]IDF)
	tf := NewAccumulator()

	for {
		select {
		case p := <-c.posting:
			for term, freq := range p.Words.Values {
				tf.Add(term, freq)
				_, present := IDFs[term]
				if !present {
					IDFs[term] = make([]IDF, 0)
				}
				IDFs[term] = append(IDFs[term], IDF{DocId: p.DocId, TF: freq})
			}
		case <-c.quit:

			for term, frequencies := range IDFs {
				// seek ahead by 0 to get current position
				filePos, _ := f.Seek(0, io.SeekCurrent)

				encoded, err := json.Marshal(frequencies)
				if err != nil {
					panic(err)
				}

				f.WriteString(string(encoded) + "\n")

				c.lexicon <- LexiconEntry{
					Term:      term,
					Frequency: tf.GetValue(term),
					FilePos:   filePos,
				}
			}
			f.Sync()
			close(c.postingDone)
			return
		}
	}
}

func buildDocMap(c *Channels) {
	defer docMapWG.Done()

	docMaps := make([]DocMap, 0)

	// TODO: allow file location to be configurable
	f, err := os.Create("./docmap.txt")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	for {
		select {
		case docMap := <-c.docmap:
			docMaps = append(docMaps, docMap)
		case <-c.postingDone:
			for _, docMap := range docMaps {
				line := fmt.Sprintf("%s %d\n", docMap.DocId, docMap.Length)
				f.WriteString(line)
			}
			f.Sync()

			return
		}
	}

}

func timestamp() time.Time {
	t := time.Now().UTC()
	return t
	//return strconv.FormatInt(t, 10)
}

func main() {
	start := timestamp()

	flag.Parse()

	// setup stoplist
	stopFunc := getStopFunc()

	// prepare corpus for parsing
	f := openFile(corpus)
	defer f.Close()

	var c Corpus
	contents, err := ioutil.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	xml.Unmarshal(contents, &c)

	channels := NewChannels()

	docMapWG = sync.WaitGroup{}
	docMapWG.Add(1)
	postingWG = sync.WaitGroup{}
	postingWG.Add(1)
	lexiconWG = sync.WaitGroup{}
	lexiconWG.Add(1)

	go buildPosting(channels)
	go buildDocMap(channels)
	go buildLexicon(channels)

	parseWG = sync.WaitGroup{}

	numDocs := 0

	for _, doc := range c.Documents {
		parseWG.Add(1)
		numDocs += 1
		go parseDocument(doc, stopFunc, channels)
	}

	parseWG.Wait()

	// signal to the remaining goroutines to flush to disk
	close(channels.quit)

	// wait on the other goroutines to write their files before exiting
	postingWG.Wait()
	lexiconWG.Wait()
	docMapWG.Wait()

	fmt.Printf("Indexed %d documents in %s\n", numDocs, time.Since(start))
}
