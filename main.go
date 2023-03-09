package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/ameshkov/dnsstamps"
	"github.com/miekg/dns"
	"github.com/schollz/progressbar/v3"
)

const (
	bootstrap    = "117.50.10.10"
	queryTimeout = time.Second * 5
)

type CheckResult struct {
	Provider string
	Stamp    string
	times    int
	elapsed  int64
}

func main() {
	stampStrings, err := readStampStrings("./resolvers.txt")
	if err != nil {
		fmt.Println(err)
		return
	}

	size := len(stampStrings)
	if size == 0 {
		fmt.Println("stamp list is empty")
		return
	}

	bootstraps := []string{bootstrap}

	results := make([]CheckResult, size)
	for i, s := range stampStrings {
		stamp, err := dnsstamps.NewServerStampFromString(s)
		if err != nil {
			fmt.Printf("failed to parse %s: %w", s, err)
			return
		}

		results[i].Provider = stamp.ProviderName
		results[i].Stamp = s
		results[i].times = 0
		results[i].elapsed = 0
	}

	for i := 0; i < 3; i++ {
		fmt.Printf("Round %d\n", i+1)

		bar := progressbar.Default(int64(size))

		var wg sync.WaitGroup
		wg.Add(size)
		for i, s := range stampStrings {
			go func(pos int, s string) {
				elapsed, err := testStamp(bootstraps, s)
				if err == nil {
					results[pos].times++
					results[pos].elapsed += elapsed
				}

				bar.Add(1)

				wg.Done()
			}(i, s)
		}
		wg.Wait()
	}

	printResults(results)
}

func readStampStrings(filename string) ([]string, error) {
	readFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	defer readFile.Close()

	fileScanner := bufio.NewScanner(readFile)

	fileScanner.Split(bufio.ScanLines)

	var results []string

	for fileScanner.Scan() {
		s := strings.Trim(fileScanner.Text(), " ")
		if s == "" {
			continue
		}

		results = append(results, s)
	}

	return results, nil
}

func testStamp(bootstraps []string, stampString string) (int64, error) {
	q := new(dns.Msg)
	q.SetQuestion("youtube.com.", dns.TypeA)

	opt := &upstream.Options{
		Bootstrap: bootstraps,
		Timeout:   queryTimeout,
		// ServerIPAddrs:      serverIPAddrs,
		InsecureSkipVerify: true,
	}

	u, err := upstream.AddressToUpstream(stampString, opt)
	if err != nil {
		return 0, err
	}
	defer u.Close()

	start := time.Now()
	r, err := u.Exchange(q)
	elapsed := time.Since(start)

	if err != nil {
		return 0, err
	}

	if len(r.Answer) == 0 {
		return 0, fmt.Errorf("no answer")
	}

	// if err == nil && len(r.Answer) > 0 {
	// 	fmt.Printf("%d|%s|%s\n", elapsed.Milliseconds(), stamp.ProviderName, s)
	// }
	return elapsed.Milliseconds(), nil
}

func printResults(results []CheckResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].times == 0 && results[j].times == 0 {
			return false
		}

		if results[i].times == 0 {
			return false
		}

		if results[j].times == 0 {
			return true
		}

		a := float64(results[i].elapsed) / float64(results[i].times)
		b := float64(results[j].elapsed) / float64(results[j].times)

		return a < b
	})

	var data []CheckResult
	if len(results) > 10 {
		data = results[:10]
	} else {
		data = results
	}

	fmt.Println("Average Elapsed|Test Times|Provider|Stamp")
	for _, r := range data {
		if r.times > 0 {
			fmt.Printf("%.2f|%d|%s|%s\n", float64(r.elapsed)/float64(r.times), r.times, r.Provider, r.Stamp)
		}
	}
}
