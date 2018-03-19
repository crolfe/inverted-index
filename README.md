# inverted-index

This project implements an inverted index, which is commonly used to provide full text search (e.g. in Elasticsearch), and is also used by databases like PostgreSQL (not only for search, but also features like the GIN index on JSON columns).

## Indexing
A corpus (or collection of documents) is indexed in the following manner:

- each document is tokenized (e.g. split on whitespace)
- the tokens are normalized by case folding, stripping newlines etc.  There are more advanced techniques such as [stemming](https://en.wikipedia.org/wiki/Stemming) and [lemmatization](https://en.wikipedia.org/wiki/Lemmatisation), which are not done in this code
- overly commond words, or "stopwords" (e.g. a, as, it, the) are removed, as they are unlikely to do anything but add noise to the search results
- Statistics about the following pieces of information are built up:
  - the number of times a term appears in the current document (IDF)
  - the length of the document 
  - the number of times a term appears across all documents (TF)
  - the average length of all documents
  - the number of documents in the the collection
  
## Represenation on disk
-Posting file: each row is a JSON encoded list of objects, which contain a document identifier, and the in document frequency (IDF)
- Document map: this is stored as a map of document id: document length 
- Lexicon: this is stored as a nested object with the outmost key is a term, which points to an object containing the trm frequency (TF), and a pointer to the documents it appears in within the posting file
- Metadata file: this is a simple JSON object containing information on the collection size, and the average document length (in words)

## Searching

This repo uses a ranked retrieval function ([BM25](https://en.wikipedia.org/wiki/Okapi_BM25)), which is intended to provide more relevant search results than a boolean search.  This controls for factors such as document length (longer documents contain more terms, but they may not necessarily be more relevant than a shorter document also containing some/all of the query terms)

Approach:

- for term in query:
  - check if term is a stopword, and skip it if so
  - check if term is in the lexicon (skip it if so), and the lookup the documents the term appears 
  - for every document the current term appears in, calculate a BM25 score and store it in an accumulator
  - repeat this process for all remaining terms, and return the document ids containing the highest accumulated BM25 score
  
  
### Usage
Compilation:
`go build`

Indexing:
`./inverted-index -action index -corpus corpus/big`

Searching:
`./inverted-index -action search -query new -query york`

 (last tested with Go 1.9.2 on Ubuntu 16.04)


