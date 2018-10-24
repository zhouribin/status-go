package rpcfilters

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	defaultCacheSize = 20
)

type cacheRecord struct {
	block uint64
	hash  common.Hash
	logs  []types.Log
}

func newCache(size int) *cache {
	return &cache{
		records: make([]cacheRecord, size),
		size:    size,
	}
}

type cache struct {
	mu      sync.RWMutex
	last    int // index of the last record
	size    int // length of the records
	records []cacheRecord
}

// add inserts logs into cache and returns added and replaced logs.
// replaced logs with will be returned with Removed=true.
func (c *cache) add(logs []types.Log) (added, replaced []types.Log, err error) {
	if len(logs) == 0 {
		return nil, nil, nil
	}
	aggregated := aggregateLogs(logs, c.size) // size doesn't change
	if len(aggregated) == 0 {
		return nil, nil, nil
	}
	if err := checkLogsAreInOrder(aggregated); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// find common block. e.g. [3,4] and [1,2,3,4] = 3
	last := c.last
	for aggregated[0].block < c.records[last].block && last > 0 {
		last--
	}
	c.records, c.last, added, replaced = merge(last, c.records, aggregated)
	return added, replaced, nil
}

func (c *cache) earliestBlockNum() uint64 {
	if len(c.records) == 0 {
		return 0
	}
	return c.records[0].block
}

func checkLogsAreInOrder(records []cacheRecord) error {
	for prev, i := 0, 1; i < len(records); i++ {
		if records[prev].block == records[i].block-1 {
			prev = i
		} else {
			return fmt.Errorf(
				"logs must be delivered straight in order. gaps between blocks '%d' and '%d'",
				records[prev].block, records[i].block,
			)
		}
	}
	return nil
}

// merge merges received records into old slice starting at provided position, example:
// [1, 2, 3, 0, 0] and position 1
//    [2, 3, 4]
// [1, 2, 3, 4, 0]
// if limit of the old slice is reached records will be popped in the fifo order, e.g:
// [1, 2, 3, 4, 0] starting at position 3
//          [4, 5, 6]
// [2, 3, 4, 5, 6]
// if hash doesn't match previously received hash - such block was removed due to reorg
// logs that were a part of that block will be returned with Removed set to true
func merge(position int, old, received []cacheRecord) ([]cacheRecord, int, []types.Log, []types.Log) {
	var (
		added, replaced []types.Log
		limit           = len(old) - 1
		next            = position
		last            = position
	)
	for i := range received {
		record := received[i]
		if last == limit && record.block > old[last].block {
			// clear space for the new record
			copy(old, old[1:])
			old[last] = record
			added = append(added, record.logs...)
		} else if old[next].hash == record.hash {
			// if record already in the cache just move forward
			last = next
			next++
		} else if (old[next].hash == common.Hash{}) {
			// nothing to replace.
			old[next] = record
			added = append(added, record.logs...)
			last = next
			next++
		} else if record.hash != old[next].hash {
			// record hash is not equal to previous record hash at the same height. reorg
			replaced = append(replaced, old[next].logs...)
			added = append(added, record.logs...)
			old[next] = record
			last = next
			next++
		}
	}
	return old, last, added, replaced
}

func aggregateLogs(logs []types.Log, limit int) []cacheRecord {
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].BlockNumber > logs[j].BlockNumber
	})
	rst := make([]cacheRecord, limit)
	pos, start := len(rst)-1, 0
	var hash common.Hash
	for i := range logs {
		log := logs[i]
		if (hash != common.Hash{}) && hash != log.BlockHash {
			rst[pos].logs = logs[start:i]
			start = i
			if pos-1 < 0 {
				break
			}
			pos--
		}
		rst[pos].logs = logs[start:]
		rst[pos].block = log.BlockNumber
		rst[pos].hash = log.BlockHash
		hash = log.BlockHash
	}
	return rst[pos:]
}
