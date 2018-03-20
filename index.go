package main

import (
	"encoding/json"
	"encoding/xml"
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
	docMapWG, parseWG, postingWG, lexiconWG, metaWG sync.WaitGroup
)

var waitGroups []sync.WaitGroup

func index() {
	start := timestamp()

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
	lexiconWG = sync.WaitGroup{}
	metaWG = sync.WaitGroup{}
	postingWG = sync.WaitGroup{}

	postingWG.Add(1)
	docMapWG.Add(1)
	lexiconWG.Add(1)
	metaWG.Add(1)

	go buildPosting(channels)
	go buildDocMap(channels)
	go buildLexicon(channels)
	go buildMetadata(channels)

	parseWG = sync.WaitGroup{}

	var numDocs int

	for _, doc := range c.Documents {
		parseWG.Add(1)
		numDocs += 1
		go parseDocument(doc, stopFunc, channels)
	}

	parseWG.Wait()

	// consumed by buildMetadata goroutine
	channels.corpusSize <- numDocs

	// signal to the buildPosting goroutine to start flushing to disk
	close(channels.parsingDone)

	// wait on the other goroutines to write their files before exiting
	postingWG.Wait()
	docMapWG.Wait()
	lexiconWG.Wait()
	metaWG.Wait()

	fmt.Printf("Indexed %d documents in %s\n", numDocs, time.Since(start))
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

	docmap := DocMap{DocId: d.Id, Length: int64(len(words))}

	// consumed by buildLexicon goroutine
	c.docmap <- docmap

	// consumed by buildMetadata goroutine
	c.docLength <- len(words)

	return
}

func buildLexicon(c *Channels) {
	defer lexiconWG.Done()

	entries := make(map[string]LexiconEntry)

	f, err := os.Create(LEXICON_FILE)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	for {
		select {
		case l := <-c.lexicon:
			entries[l.Term] = l.Entry
		case <-c.postingDone:
			encoded, jerr := json.Marshal(entries)

			if jerr != nil {
				panic(jerr)
			}
			f.WriteString(string(encoded))
			f.Sync()
			return
		}
	}
}

func buildPosting(c *Channels) {
	defer postingWG.Done()

	f, err := os.Create(POSTING_FILE)
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
		case <-c.parsingDone:

			for term, frequencies := range IDFs {
				// seek ahead by 0 to get current position
				filePos, _ := f.Seek(0, io.SeekCurrent)

				encoded, err := json.Marshal(frequencies)
				if err != nil {
					panic(err)
				}

				f.WriteString(string(encoded) + "\n")

				c.lexicon <- *NewLexicon(term, tf.GetValue(term), filePos)
			}
			f.Sync()
			close(c.postingDone)
			return
		}
	}
}

func buildDocMap(c *Channels) {
	defer docMapWG.Done()

	docMap := make(map[string]int64)

	// TODO: allow file location to be configurable
	f, err := os.Create(DOC_MAP_FILE)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	for {
		select {
		case d := <-c.docmap:
			docMap[d.DocId] = d.Length
		case <-c.postingDone:
			encoded, jerr := json.Marshal(docMap)

			if jerr != nil {
				panic(jerr)
			}

			f.WriteString(string(encoded))
			f.Sync()

			return
		}
	}
}

func buildMetadata(c *Channels) {
	defer metaWG.Done()
	f, ferr := os.Create(METADATA_FILE)

	if ferr != nil {
		panic(ferr)
	}

	lengthAccumulator := 0
	var corpusSize int

	for {
		select {
		case l := <-c.docLength:
			lengthAccumulator += l
		case cs := <-c.corpusSize:
			corpusSize = cs
		case <-c.postingDone:
			avg := int(lengthAccumulator / corpusSize)
			m := Metadata{AverageLength: avg, CorpusSize: corpusSize}
			encoded, jerr := json.Marshal(m)

			if jerr != nil {
				panic(jerr)
			}

			f.WriteString(string(encoded))
			f.Sync()

			return
		}
	}

}
