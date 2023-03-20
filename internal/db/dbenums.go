package db

type SwapDirection int8

// Do not change these constantss. SQL Queries may assume this value dirrectly.
const (
	RuneToAsset   SwapDirection = 0
	AssetToRune   SwapDirection = 1
	RuneToSynth   SwapDirection = 2
	SynthToRune   SwapDirection = 3
	RuneToDerived SwapDirection = 4
	DerivedToRune SwapDirection = 5
)
