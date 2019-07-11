// Package dynhist implements dynamic histogram collector.
package dynhist

import (
	"fmt"
	"sync"
)

const DefaultMaxBuckets = 20

type Collector struct {
	sync.Mutex
	MaxBuckets int
	Min        float64
	Max        float64
	Count      int64
	Buckets    []Bucket
}

type Bucket struct {
	Min   float64
	Max   float64
	Count int64
}

func (c *Collector) Add(v float64) {
	c.Lock()
	defer func() {
		if len(c.Buckets) > c.MaxBuckets {
			//minSumCount := c.Count
			minWidth := c.Max - c.Min
			mergePoint := 0
			for i := 1; i < len(c.Buckets); i++ {
				//count := c.Buckets[i-1].Count + c.Buckets[i].Count
				//if count < minSumCount {
				//	minSumCount = count
				//	mergePoint = i
				//}
				width := c.Buckets[i].Max - c.Buckets[i-1].Min
				if width < minWidth {
					minWidth = width
					mergePoint = i
				}
			}
			//fmt.Printf("merging %v and %v\n", c.Buckets[mergePoint-1], c.Buckets[mergePoint])
			merged := Bucket{
				Count: c.Buckets[mergePoint-1].Count + c.Buckets[mergePoint].Count,
				Min:   c.Buckets[mergePoint-1].Min,
				Max:   c.Buckets[mergePoint].Max,
			}
			c.Buckets = append(c.Buckets[:mergePoint-1], c.Buckets[mergePoint:]...)
			c.Buckets[mergePoint-1] = merged
		}
		c.Unlock()
	}()

	c.Count++

	if len(c.Buckets) == 0 {
		if c.MaxBuckets == 0 {
			c.MaxBuckets = DefaultMaxBuckets
		}
		c.Buckets = make([]Bucket, 1, c.MaxBuckets)
		c.Buckets[0].Min = v
		c.Buckets[0].Max = v
		c.Buckets[0].Count = 1

		c.Min = v
		c.Max = v

		return
	}

	if v < c.Min {
		c.Buckets = append([]Bucket{{Count: 1, Min: v, Max: v}}, c.Buckets...)
		c.Min = 0
		return
	}

	if v > c.Max {
		c.Buckets = append(c.Buckets, Bucket{Count: 1, Min: v, Max: v})
		c.Max = v
		return
	}

	//  [1 3] [4 4] 5 [7 9]
	for i, b := range c.Buckets {
		if v >= b.Min {
			if v <= b.Max {
				c.Buckets[i].Count++
				return
			}
		} else {
			// Insert new bucket.
			c.Buckets = append(c.Buckets, Bucket{})
			copy(c.Buckets[i+1:], c.Buckets[i:])
			c.Buckets[i] = Bucket{Count: 1, Min: v, Max: v}
			return
		}
	}
}

func (c *Collector) String() string {
	c.Lock()
	defer c.Unlock()

	res := ""
	for _, b := range c.Buckets {
		res += fmt.Sprintf("%8.2f %8.2f %6d %5.2f%%\n", b.Min, b.Max, b.Count, float64(100*b.Count)/float64(c.Count))
	}
	return res
}
