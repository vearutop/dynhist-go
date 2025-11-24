// Package main implements a tool to count distribution of values received from STDIN.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/bool64/dev/version"
	"github.com/vearutop/dynhist-go"
)

func main() {
	buckets := flag.Int("buckets", 10, "Number of buckets.")
	ver := flag.Bool("version", false, "Print version.")

	flag.Parse()

	if *ver {
		fmt.Println(version.Info().Version)
		return
	}

	hist := dynhist.Collector{
		BucketsLimit: *buckets,
		WeightFunc:   dynhist.ExpWidth(1.2, 1),
		PrintSum:     true,
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if f, err := strconv.ParseFloat(scanner.Text(), 64); err == nil {
			hist.Add(f)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err.Error())
	}

	for _, p := range []float64{99.9, 99, 90, 75, 50} {
		percentile := hist.Percentile(p)
		pFmt := "%.2f"

		if isInt(percentile) {
			pFmt = "%.0f"
		}

		println(fmt.Sprintf("%.1f%% < "+pFmt+", sum < "+pFmt, p, hist.Percentile(p), hist.PercentileSum(p)))
	}

	println()
	println(hist.String())
}

func isInt(f float64) bool {
	return f == float64(int(f))
}
