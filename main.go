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
	"strconv"
	"strings"
	"time"
)

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
	max, min int
	sens     int
	ratio    int
	dryrun   bool
	debug    bool
	wait     time.Duration
}

func (p *poller) poll() {
	maxIn := p.ratio * 100
	blMin := p.sens * p.max / 100
	for {
		blight, err := sysfileReadInt(backlightCurrPath)
		if err != nil {
			log.Fatal("cannot get backlight value: ", err)
		}
		inlight, err := sysfileReadInt(illuminancePath)
		if err != nil {
			log.Fatal("cannot get ambient light value: ", err)
		}
		inlightPercent := inlight * 100 / maxIn
		if inlightPercent > 100 {
			inlightPercent = 100
		}
		nblight := inlightPercent*p.max/100 + p.min
		if nblight > p.max {
			nblight = p.max
		}
		diff := nblight - blight
		if diff < 0 {
			diff = -diff
		}
		if p.debug {
			log.Printf("light = %d (%d%%), back-light = %d, set %d (diff %d, min %d)", inlight, inlightPercent, blight, nblight, diff, blMin)
		}
		// Set backlight if there is more than the minimum change thresold to adjust. Or if we are below min (level was never set.)
		if diff >= blMin || blight < p.min {
			if !p.dryrun {
				if err := sysfileWriteInt(backlightCurrPath, nblight); err != nil {
					log.Fatal("cannot set backlight: ", err)
				}
			} else {
				log.Printf("change backlight to %d%%; illuminance = %d, backlight = %d (was %d)", inlightPercent, inlight, nblight, blight)
			}
		}
		time.Sleep(p.wait)
	}
}

func main() {
	max, err := sysfileReadInt(backlightMaxPath)
	if err != nil {
		log.Fatal("cannot get backlight max value: ", err)
	}
	p := poller{
		min:    75,  // backlight min N
		max:    max, // backlight max N
		sens:   2,   // sensitivity %
		ratio:  60,  // lux = 1%
		dryrun: false,
		debug:  false,
		wait:   2 * time.Second,
	}
	flag.IntVar(&p.min, "min", p.min, "Minimum value `N` for backlight")
	flag.IntVar(&p.max, "max", p.max, "Maximum value `N` for backlight (autodetected)")
	flag.IntVar(&p.sens, "sensitivity", p.sens, "Minimum amount `S` in percent of backlight change to perform")
	flag.IntVar(&p.ratio, "ratio", p.ratio, "Ratio `R` of light change: number of lux for a 1% change in backlight")
	flag.BoolVar(&p.dryrun, "dryrun", p.dryrun, "Do not set backlight, only print what would happen")
	flag.BoolVar(&p.debug, "debug", p.debug, "Print values read from sensors every wait duration")
	flag.DurationVar(&p.wait, "wait", p.wait, "Duration `T` between checks for changed light conditions")
	flag.Parse()
	p.poll()
}
