package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	action, corpus, stoplist string
)

const LEXICON_FILE = "lexicon.json"
const POSTING_FILE = "posting.json"
const DOC_MAP_FILE = "docmap.json"
const METADATA_FILE = "meta.json"

type queryterms []string

func (q *queryterms) String() string {
	return fmt.Sprintf("%s", *q)
}
func (q *queryterms) Set(value string) error {
	*q = append(*q, value)
	return nil
}

var query queryterms

func init() {
	flag.StringVar(&action, "action", "", "Valid options: index, search")
	flag.Var(&query, "query", "Add additonal -query flags per query term")
	flag.StringVar(&corpus, "corpus", "", "<corpus>")
	flag.StringVar(&stoplist, "stoplist", "", "<stoplist>")
}

func openFile(filename string) (f *os.File) {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening", filename)
		panic(err)
	}
	return
}

func getStopFunc() func(s string) bool {
	f := openFile(stoplist)
	defer f.Close()

	s := bufio.NewScanner(f)

	stopList := make(map[string]bool)

	for s.Scan() {
		word := strings.TrimSpace(s.Text())
		// ignore the bool value, as we are using this map like a set
		stopList[word] = true
	}

	return func(s string) (present bool) {
		_, present = stopList[s]
		return
	}
}

func timestamp() time.Time {
	return time.Now().UTC()
}

func main() {
	flag.Parse()

	if stoplist == "" {
		stoplist = "stoplist"
	}
	if action == "index" {
		if corpus == "" {
			fmt.Println("You must set the -corpus parameter")
			os.Exit(1)
		}
		index()
		return
	} else if action == "search" {
		if len(query) == 0 {
			fmt.Println("You must set the -query paramter")
			os.Exit(1)
		}
		cmdSearch(query)
		return
	}

	if action == "" {
		fmt.Println("You must set the -action parameter")
	} else {
		fmt.Println(action, "is not a valid action")
	}
	os.Exit(1)
}
