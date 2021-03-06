// Copyright 2015 Giulio Iotti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	dlog *log.Logger // debug logger
	vlog *log.Logger // verbose log
	elog *log.Logger // error log
)

func init() {
	runtime.GOMAXPROCS(1)
}

const (
	illuminancePath   = "/sys/bus/iio/devices/iio:device0/in_illuminance_raw"
	backlightMaxPath  = "/sys/class/backlight/intel_backlight/max_brightness"
	backlightCurrPath = "/sys/class/backlight/intel_backlight/brightness"
)

func sysfileReadInt(f string) (int, error) {
	file, err := os.Open(f)
	if err != nil {
		return -1, err
	}
	defer file.Close()
	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return -1, err
	}
	text := strings.TrimSpace(string(buf))
	return strconv.Atoi(text)
}

func sysfileWriteInt(name string, n int) error {
	buf := fmt.Sprintf("%d\n", n)
	return ioutil.WriteFile(name, []byte(buf), 0644)
}

type poller struct {
	probes   int
	max, min int
	grad     int
	sens     int
	ratio    int
	dryrun   bool
	debug    bool
	wait     time.Duration
	gradWait time.Duration
}

func (p *poller) poll() {
	var inIndex int
	inlights := make([]int, p.probes)
	granularity := 100
	maxIn := p.ratio * granularity
	for {
		var inlight int
		var err error
		blight, err := sysfileReadInt(backlightCurrPath)
		if err != nil {
			elog.Fatal("cannot get backlight value: ", err)
		}
		for {
			inlight, err = sysfileReadInt(illuminancePath)
			if err != nil {
				elog.Fatal("cannot get ambient light value: ", err)
			}
			inlights[inIndex] = inlight
			inIndex = (inIndex + 1) % cap(inlights)
			if inIndex == 0 {
				break
			}
		}
		inlight = 0
		n := 0
		// Average light in the last inlights probes
		for i := 0; i < len(inlights); i++ {
			inlight += inlights[i]
		}
		if n > 0 {
			inlight = inlight / n
		}
		inlightPercent := inlight * granularity / maxIn
		if inlightPercent > granularity {
			inlightPercent = granularity
		}
		nblight := inlightPercent*p.max/granularity + p.min
		if nblight > p.max {
			nblight = p.max
		}
		diff := nblight - blight
		if diff < 0 {
			diff = -diff
		}
		dlog.Printf("light = %d (%d%%), back-light = %d, set %d (diff %d, min-diff %d)", inlight, inlightPercent, blight, nblight, diff, p.sens)
		// Set backlight if there is more than the minimum change thresold to adjust. Or if we are below min (level was never set.)
		if diff >= p.sens || blight < p.min {
			vlog.Printf("change backlight to %d%%; illuminance = %d, backlight = %d (was %d)", inlightPercent, inlight, nblight, blight)
			if !p.dryrun {
				if err := p.setBlight(blight, nblight); err != nil {
					elog.Fatal("cannot set backlight: ", err)
				}
			}
			continue // When light was changed, probe again right away
		}
		time.Sleep(p.wait)
	}
}

// Make a simple transition between backlight levels
func (p *poller) setBlight(curr, set int) error {
	// Decrease
	if curr > set {
		for curr > set {
			curr -= p.grad
			if curr < set {
				curr = set
			}
			if err := sysfileWriteInt(backlightCurrPath, curr); err != nil {
				return err
			}
			time.Sleep(p.gradWait)
		}
		return nil
	}
	// Increase
	for curr < set {
		curr += p.grad
		if curr > set {
			curr = set
		}
		if err := sysfileWriteInt(backlightCurrPath, curr); err != nil {
			return err
		}
		time.Sleep(p.gradWait)
	}
	return nil
}

func main() {
	p := poller{
		probes:   8,
		grad:     5,  // how much to change backlight for gradual change
		min:      40, // backlight min N
		max:      0,  // backlight max N (0 = autodetect)
		sens:     18, // sensitivity %
		ratio:    20, // lux = 1%
		dryrun:   false,
		debug:    false,
		wait:     4 * time.Second,
		gradWait: 200 * time.Millisecond,
	}
	flag.IntVar(&p.grad, "animation-steps", p.grad, "Number `N` of backlight to add or remove to smoothly change backlight")
	flag.IntVar(&p.probes, "probes", p.probes, "Number `N` of illuminance probes to average")
	flag.IntVar(&p.min, "min", p.min, "Minimum value `N` for backlight")
	flag.IntVar(&p.max, "max", p.max, "Maximum value `N` for backlight (0 = autodetected)")
	flag.IntVar(&p.sens, "sensitivity", p.sens, "Minimum amount `S` in percent of backlight change to perform")
	flag.IntVar(&p.ratio, "ratio", p.ratio, "Ratio `R` of light change: number of lux for a 1% change in backlight")
	flag.BoolVar(&p.dryrun, "dryrun", p.dryrun, "Do not set backlight, only print what would happen")
	flag.BoolVar(&p.debug, "debug", p.debug, "Print values read from sensors every wait duration")
	flag.DurationVar(&p.wait, "wait", p.wait, "Duration `T` between checks for changed light conditions")
	flag.DurationVar(&p.gradWait, "animation", p.gradWait, "Duration `T` for smooth animation on light change")
	flag.Parse()
	dlogOut := ioutil.Discard
	vlogOut := ioutil.Discard
	elogOut := os.Stderr
	if p.dryrun {
		vlogOut = os.Stdout
	}
	if p.debug {
		dlogOut = os.Stdout
		vlogOut = os.Stdout
	}
	elog = log.New(elogOut, "bls: error: ", log.LstdFlags)
	dlog = log.New(dlogOut, "bls: debug: ", log.LstdFlags)
	vlog = log.New(vlogOut, "bls: info: ", log.LstdFlags)
	if p.max == 0 {
		var err error
		p.max, err = sysfileReadInt(backlightMaxPath)
		if err != nil {
			elog.Fatal("cannot get backlight max value: ", err)
		}
	}
	p.poll()
}
