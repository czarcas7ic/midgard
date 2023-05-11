package db

type EventLocation int

const (
	BeginBlockEvents EventLocation = iota
	TxsResults
	EndBlockEvents
)

type EventId struct {
	BlockHeight int64
	Location    EventLocation
	TxIndex     int
	EventIndex  int
}

// EventIds are encoded as int64s in the following decimal format:
// For BeginBlockEvents:
//  h,hhh,hhh,hh0,eee,eee,eee
// For TxsResults:
//  h,hhh,hhh,hh[1-8],ttt,tte,eee
// For EndBlockEvents:
//  h,hhh,hhh,hh9,eee,eee,eee
// where:
//  h = block height
//  t = tx index
//  e = event index
// This allows for:
// Almost a billion blocks, which is enough for 150 years at 5 second block time.
// A billion events in `begin_block_events` and `end_block_events`.
// 800,000 transactions per block, and 10000 events per transaction.

const (
	blockHeightScale = 1e10
	typeDigit        = 1e9
	txIndexScale     = 1e4
	endBlockPseudoTx = 999999
)

func (e EventId) AsBigint() int64 {
	switch e.Location {
	case BeginBlockEvents:
		return e.BlockHeight*blockHeightScale + 0*typeDigit + int64(e.EventIndex)
	case TxsResults:
		return e.BlockHeight*blockHeightScale + 1*typeDigit + int64(e.TxIndex)*txIndexScale +
			int64(e.EventIndex)
	case EndBlockEvents:
		return e.BlockHeight*blockHeightScale + 9*typeDigit + int64(e.EventIndex)
	default:
		panic("invalid event location")
	}
}

// Opposite of AsBigint.
func ParseEventId(eid int64) (res EventId) {
	res.BlockHeight = eid / blockHeightScale
	eid %= blockHeightScale
	switch {
	case eid < typeDigit:
		res.Location = BeginBlockEvents
		res.TxIndex = 0
		res.EventIndex = int(eid % typeDigit)
	case eid < 9*typeDigit:
		res.Location = TxsResults
		res.TxIndex = int((eid - typeDigit) / txIndexScale)
		res.EventIndex = int(eid % txIndexScale)
	default:
		res.Location = EndBlockEvents
		res.TxIndex = endBlockPseudoTx
		res.EventIndex = int(eid % typeDigit)
	}
	return
}

func HeightFromEventId(eid int64) int64 {
	return eid / blockHeightScale
}

func HeightToEventId(height uint64) uint64 {
	return height * blockHeightScale
}
