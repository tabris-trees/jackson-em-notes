package fieldrenderer

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/euphoricrhino/jackson-em-notes/go/pkg/heatmap"
)

// Options represents options to run the field renderer.
type Options struct {
	HeatMapFile string
	OutputFile  string
	// Gamma correction to be applied to heatmap.
	Gamma  float64
	Width  int
	Height int
	Field  func(x, y int) float64
}

// Run runs the field renderer with the given options.
func Run(opts Options) error {
	hm, err := heatmap.Load(opts.HeatMapFile, opts.Gamma)
	if err != nil {
		return err
	}

	data := make([]float64, opts.Width*opts.Height)
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(workers)
	cnt := int32(0)
	for w := 0; w < workers; w++ {
		go func(i int) {
			defer wg.Done()
			for x := 0; x < opts.Width; x++ {
				if x%workers != i {
					continue
				}
				for y := 0; y < opts.Height; y++ {
					data[y*opts.Width+x] = opts.Field(x, y)
					atomic.AddInt32(&cnt, 1)
				}
			}
		}(w)
	}
	// Progress counter.
	counterDone := make(chan struct{})
	go func() {
		erase := strings.Repeat(" ", 80)
		nextMark := 1.0
		for {
			doneCnt := int(atomic.LoadInt32(&cnt))
			if doneCnt == opts.Width*opts.Height {
				fmt.Printf("\r%v\rrendering complete\n", erase)
				close(counterDone)
				return
			}
			progress := float64(doneCnt) / float64(opts.Width*opts.Height) * 100.0
			if progress >= nextMark {
				fmt.Printf("\r%v\rrendering... %.2f%% done", erase, progress)
				nextMark = math.Ceil(progress)
			}
			runtime.Gosched()
		}
	}()
	wg.Wait()
	<-counterDone

	// Normalize the data.
	max, min := data[0], data[0]
	for i := 1; i < len(data); i++ {
		if max < data[i] {
			max = data[i]
		}
		if min > data[i] {
			min = data[i]
		}
	}
	spread := max - min
	if spread >= 1e-8 {
		for i := range data {
			data[i] = (data[i] - min) / spread
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, opts.Width, opts.Height))
	for x := 0; x < opts.Width; x++ {
		for y := 0; y < opts.Height; y++ {
			pixel := data[y*opts.Width+x]
			pos := int(pixel * float64(len(hm)-1))
			r, g, b, a := hm[pos].RGBA()
			img.SetRGBA64(x, y, color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(a)})
		}
	}
	out, err := os.Create(opts.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file '%v': %v", opts.OutputFile, err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		return fmt.Errorf("failed to encode to PNG: %v", err)
	}
	return nil
}