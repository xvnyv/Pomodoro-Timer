// +build cgo
package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

// This cgo solution for flushing stdin in go was inspired by the solution presented here: https://github.com/odeke-em/drive/issues/157
/*
#include <termios.h>
#include <unistd.h>
void flush_tty_in() {
	if (isatty(0))
		tcflush(0, TCIFLUSH);
}
*/
import "C"

const (
	workingDuration   time.Duration = 25 * time.Minute * 1 / 60 / 5
	restingDuration   time.Duration = 5 * time.Minute * 1 / 60
	longBreakDuration time.Duration = 15 * time.Minute * 1 / 60
)

//go:embed sounds/Happyday.mp3
var f embed.FS

// flushTTYin flushes the input buffer of the tty.
// Use it before asking the user to make decisions, especially after
// a long wait.
func flushTTYin() {
	C.flush_tty_in()
}

func padInt(i int, length int) string {
	s := strconv.Itoa(i)
	if len(s) >= length {
		return s
	}
	return strings.Repeat("0", length-len(s)) + s
}

func parseDuration(d time.Duration) string {
	minutes := math.Floor(d.Minutes())
	seconds := math.Floor(d.Seconds() - (minutes * 60))
	return padInt(int(minutes), 2) + ":" + padInt(int(seconds), 2)
}

func initializeSound() *beep.Ctrl {
	file, err := f.Open("sounds/Happyday.mp3")
	if err != nil {
		log.Fatal(err)
	}

	streamer, format, err := mp3.Decode(file)
	if err != nil {
		log.Fatal(err)
	}

	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	buffer := beep.NewBuffer(format)
	buffer.Append(streamer)
	streamer.Close()

	alarmStreamer := buffer.Streamer(0, buffer.Len())
	alarm := &beep.Ctrl{Streamer: beep.Loop(-1, alarmStreamer), Paused: false}

	return alarm
}

func main() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var timer *time.Timer
	var curDuration time.Duration

	var end time.Time
	period := "Work"
	round := 0
	cycles := 0

	fmt.Println("\nUse Ctrl-c to end the timer")

	alarm := initializeSound()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\b\b  \nStopping Pomodoro timer...\n")
		fmt.Printf("Completed cycles: %.2f\n", float64(cycles)+float64(round)/4)
		os.Exit(0)
	}()

	for {
		switch period {
		case "Work":
			fmt.Printf("\nCycle %v Round %v\n", cycles+1, round+1)
			curDuration = workingDuration
		case "Rest":
			curDuration = restingDuration
		case "Long Rest":
			curDuration = longBreakDuration
		}

		end = time.Now().Add(curDuration)
		timer = time.NewTimer(curDuration)

	TimerLoop:
		for {
			select {
			case <-timer.C:
				// play alarm
				if !alarm.Paused {
					// initial alarm
					speaker.Play(alarm)
				} else {
					// subsequent alarms
					alarm.Paused = false
				}
				// flush stdin before listening for <enter> to prevent accidental <enter> presses from stopping the alarm
				// note: only works on unix
				flushTTYin()
				fmt.Printf("\r%v is done! Press enter to continue", period)

				// update round info and set next period
				switch period {
				case "Work":
					round++
					if round == 4 {
						period = "Long Rest"
						round = 0
						cycles++
					} else {
						period = "Rest"
					}
				case "Rest", "Long Rest":
					period = "Work"
				}
				// stop alarm
				fmt.Scanln()

				speaker.Lock()
				alarm.Paused = true
				speaker.Unlock()

				// start next period
				break TimerLoop
			case t := <-ticker.C:
				// display time left for current period
				fmt.Printf("\r%v Period: %v", period, parseDuration(end.Sub(t)))
			}
		}
	}
}
