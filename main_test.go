package main

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

type bucket struct {
	upperBound float64
	count      float64
}

// buckets implements sort.Interface.
type buckets []bucket

func (b buckets) Len() int           { return len(b) }
func (b buckets) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b buckets) Less(i, j int) bool { return b[i].upperBound < b[j].upperBound }

func TestSmts(t *testing.T) {
	fmt.Println("sup")

	q := 0.99
	bks := []bucket{
		{upperBound: 0.01, count: 900_000},          // 0
		{upperBound: 0.05, count: 1_800_000},        // 1
		{upperBound: 0.1, count: 2_250_000},         // 2
		{upperBound: 0.2, count: 2_550_000},         // 3
		{upperBound: 0.3, count: 2_790_000},         // 4
		{upperBound: 0.5, count: 2_910_000},         // 5
		{upperBound: 1, count: 2_970_000},           // 6
		{upperBound: 2, count: 2_982_000},           // 7
		{upperBound: 5, count: 2_983_500},           // 8
		{upperBound: math.Inf(1), count: 2_983_800}, // 9
	}

	quantile := bucketQuantile(q, bks)

	fmt.Println("quantile: ", quantile)
}

func bucketQuantile(q float64, buckets buckets) float64 {
	if q < 0 {
		return math.Inf(-1)
	}
	if q > 1 {
		return math.Inf(+1)
	}
	sort.Sort(buckets)
	if !math.IsInf(buckets[len(buckets)-1].upperBound, +1) {
		return math.NaN()
	}

	buckets = coalesceBuckets(buckets)
	ensureMonotonic(buckets)

	if len(buckets) < 2 {
		return math.NaN()
	}
	observations := buckets[len(buckets)-1].count
	if observations == 0 {
		return math.NaN()
	}
	rank := q * observations
	b := sort.Search(len(buckets)-1, func(i int) bool { return buckets[i].count >= rank })

	if b == len(buckets)-1 {
		return buckets[len(buckets)-2].upperBound
	}
	if b == 0 && buckets[0].upperBound <= 0 {
		return buckets[0].upperBound
	}
	var (
		bucketStart float64
		bucketEnd   = buckets[b].upperBound
		count       = buckets[b].count
	)
	if b > 0 {
		bucketStart = buckets[b-1].upperBound
		count -= buckets[b-1].count
		rank -= buckets[b-1].count
	}

	fmt.Println("quantile: ", bucketStart+(bucketEnd-bucketStart)*(rank/count))
	return bucketStart + (bucketEnd-bucketStart)*(rank/count)
}

// coalesceBuckets merges buckets with the same upper bound.
//
// The input buckets must be sorted.
func coalesceBuckets(buckets buckets) buckets {
	last := buckets[0]
	i := 0
	for _, b := range buckets[1:] {
		if b.upperBound == last.upperBound {
			last.count += b.count
		} else {
			buckets[i] = last
			last = b
			i++
		}
	}
	buckets[i] = last
	return buckets[:i+1]
}

// The assumption that bucket counts increase monotonically with increasing
// upperBound may be violated during:
//
//   - Recording rule evaluation of histogram_quantile, especially when rate()
//     has been applied to the underlying bucket timeseries.
//   - Evaluation of histogram_quantile computed over federated bucket
//     timeseries, especially when rate() has been applied.
//
// This is because scraped data is not made available to rule evaluation or
// federation atomically, so some buckets are computed with data from the
// most recent scrapes, but the other buckets are missing data from the most
// recent scrape.
//
// Monotonicity is usually guaranteed because if a bucket with upper bound
// u1 has count c1, then any bucket with a higher upper bound u > u1 must
// have counted all c1 observations and perhaps more, so that c  >= c1.
//
// Randomly interspersed partial sampling breaks that guarantee, and rate()
// exacerbates it. Specifically, suppose bucket le=1000 has a count of 10 from
// 4 samples but the bucket with le=2000 has a count of 7 from 3 samples. The
// monotonicity is broken. It is exacerbated by rate() because under normal
// operation, cumulative counting of buckets will cause the bucket counts to
// diverge such that small differences from missing samples are not a problem.
// rate() removes this divergence.)
//
// bucketQuantile depends on that monotonicity to do a binary search for the
// bucket with the φ-quantile count, so breaking the monotonicity
// guarantee causes bucketQuantile() to return undefined (nonsense) results.
//
// As a somewhat hacky solution until ingestion is atomic per scrape, we
// calculate the "envelope" of the histogram buckets, essentially removing
// any decreases in the count between successive buckets.
func ensureMonotonic(buckets buckets) {
	max := buckets[0].count
	for i := 1; i < len(buckets); i++ {
		switch {
		case buckets[i].count > max:
			max = buckets[i].count
		case buckets[i].count < max:
			buckets[i].count = max
		}
	}
}
