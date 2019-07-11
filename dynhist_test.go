package dynhist_test

import (
	"github.com/vearutop/dynhist"
	"testing"
)

func TestCollector_Add(t *testing.T) {
	c := &dynhist.Collector{
		MaxBuckets: 5,
	}
	for i := 0; i < 100; i++ {
		c.Add(float64(i))
		c.Add(float64(100 - i))
	}

	println(c.String())
}
