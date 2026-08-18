package search

import (
	internalText "github.com/mrzack99s/cocodb/internal/text"
)

type InvertedIndex = internalText.InvertedIndex
type SearchResult = internalText.SearchResult

func NewInvertedIndex() *InvertedIndex {
	return internalText.NewInvertedIndex()
}

func Tokenize(text string) []string {
	return internalText.Tokenize(text)
}
