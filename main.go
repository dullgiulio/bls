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
	ratio    int
	dryrun   bool
	wait     time.Duration
}

func (p *poller) poll() {
	maxIn := p.ratio * 100
	blMin := p.max / 50 // TODO: This is quite a magic number
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
		if diff >= blMin {
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
		min:    65,
		max:    max,
		ratio:  30,    // lux = 1%
		dryrun: false, // TODO
		wait:   1 * time.Second,
	}
	flag.IntVar(&p.min, "min", p.min, "Minimum value `N` for backlight")
	flag.IntVar(&p.max, "max", p.max, "Maximum value `N` for backlight (autodetected)")
	flag.IntVar(&p.ratio, "ratio", p.ratio, "Ratio `R` of light change: number of lux for a 1% change in backlight")
	flag.BoolVar(&p.dryrun, "dryrun", p.dryrun, "Do not set backlight, only print what would happen")
	flag.DurationVar(&p.wait, "wait", p.wait, "Duration `T` between checks for changed light conditions")
	flag.Parse()
	p.poll()
}
