package main

import (
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

type sysfile struct {
	*os.File
	name string
}

func newSysfile(f string) (*sysfile, error) {
	file, err := os.Open(f)
	return &sysfile{file, f}, err
}

func (f *sysfile) readInt() (int, error) {
	buf, err := ioutil.ReadAll(f.File)
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

func sysfileVal(f string) (int, error) {
	sfile, err := newSysfile(f)
	if err != nil {
		return -1, err
	}
	defer sfile.Close()
	val, err := sfile.readInt()
	if err != nil {
		return -1, err
	}
	return val, nil
}

type poller struct {
	max, ratio int
	wait       time.Duration
	dryrun     bool
}

func (p *poller) poll() {
	maxIn := p.ratio * 100
	for {
		blight, err := sysfileVal(backlightCurrPath)
		if err != nil {
			log.Fatal("cannot get backlight value: ", err)
		}
		blightPercent := blight * 100 / p.max
		fmt.Printf("<<< %d %d%%\n", blight, blightPercent)
		inlight, err := sysfileVal(illuminancePath)
		if err != nil {
			log.Fatal("cannot get ambient light value: ", err)
		}
		inlightPercent := inlight * 100 / maxIn
		if inlightPercent > 100 {
			inlightPercent = 100
		}
		fmt.Printf(">>> %d %d%%\n", inlight, inlightPercent)
		nblight := blightPercent * p.max / 100
		diff := nblight - blight
		if diff >= 5 {
			fmt.Printf("+++ %d %d%%\n", nblight, nblight*100/p.max)
			if !p.dryrun {
				if err := sysfileWriteInt(backlightCurrPath, nblight); err != nil {
					log.Print("cannot set backlight: ", err)
				}
			}
		}
		time.Sleep(p.wait)
	}
}

func main() {
	max, err := sysfileVal(backlightMaxPath)
	if err != nil {
		log.Fatal("cannot get backlight max value: ", err)
	}
	p := poller{
		max:    max,
		ratio:  30,   // lux = 1% ( + offset as detected...)
		dryrun: true, // TODO
		wait:   1 * time.Second,
	}
	p.poll()
}
