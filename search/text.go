package search

import (
	internalText "cocodb/internal/text"
)

type InvertedIndex = internalText.InvertedIndex
type SearchResult = internalText.SearchResult

func NewInvertedIndex() *InvertedIndex {
	return internalText.NewInvertedIndex()
}

func Tokenize(text string) []string {
	return internalText.Tokenize(text)
}
