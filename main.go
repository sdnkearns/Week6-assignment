package main

import (
	"fmt"
	imageprocessing "goroutines_pipeline/image_processing"
	"image"
	"os"
	"strings"
	"sync"
	"time"
)

type Job struct {
	InputPath string
	Image     image.Image
	OutPath   string
	Err       error
	Timings   StageTimings
}

type StageTimings struct {
	Load   time.Duration
	Resize time.Duration
	Gray   time.Duration
	Save   time.Duration
}

type BenchmarkResult struct {
	Mode       string
	TotalJobs  int
	Succeeded  int
	Failed     int
	Elapsed    time.Duration
	PerStage   map[string]time.Duration
	Throughput float64
}

func (r BenchmarkResult) Print() {
	fmt.Printf("\n=== Benchmark: %s ===\n", r.Mode)
	fmt.Printf("  Jobs         : %d total, %d ok, %d failed\n", r.TotalJobs, r.Succeeded, r.Failed)
	fmt.Printf("  Wall time    : %v\n", r.Elapsed.Round(time.Millisecond))
	fmt.Printf("  Throughput   : %.2f jobs/sec\n", r.Throughput)
	fmt.Println("  Stage totals (summed across all jobs):")
	for _, stage := range []string{"load", "resize", "grayscale", "save"} {
		fmt.Printf("    %-10s : %v\n", stage, r.PerStage[stage].Round(time.Microsecond))
	}
}

func loadImage(paths []string) <-chan Job {
	out := make(chan Job)
	go func() {
		// For each input path create a job and add it to
		// the out channel
		for _, p := range paths {
			job := Job{InputPath: p,
				OutPath: strings.Replace(p, "images/", "images/output/", 1)}

			if p == "" {
				job.Err = fmt.Errorf("Image load failed. Empty file path")
				out <- job
				continue
			}

			if _, err := os.Stat(p); os.IsNotExist(err) {
				job.Err = fmt.Errorf("image load failed: file does not exist %q", p)
				out <- job
				continue
			}

			t0 := time.Now()
			img := imageprocessing.ReadImage(p)
			job.Timings.Load = time.Since(t0)

			if img == nil {
				job.Err = fmt.Errorf("Image load failed: ReadImage returned nil for %q", p)
				out <- job
				continue
			}

			job.Image = img
			out <- job
		}
		close(out)
	}()
	return out
}

func resize(input <-chan Job) <-chan Job {
	out := make(chan Job)
	go func() {
		// For each input job, create a new job after resize and add it to
		// the out channel
		for job := range input { // Read from the channel
			if job.Err == nil {
				t0 := time.Now()
				job.Image = imageprocessing.Resize(job.Image)
				job.Timings.Resize = time.Since(t0)
			}
			out <- job
		}
		close(out)
	}()
	return out
}

func convertToGrayscale(input <-chan Job) <-chan Job {
	out := make(chan Job)
	go func() {
		for job := range input { // Read from the channel
			if job.Err == nil {
				t0 := time.Now()
				job.Image = imageprocessing.Grayscale(job.Image)
				job.Timings.Gray = time.Since(t0)
			}
			out <- job
		}
		close(out)
	}()
	return out
}

func saveImage(input <-chan Job) <-chan Job {
	out := make(chan Job)
	go func() {
		for job := range input { // Read from the channel
			if job.Err != nil {
				fmt.Printf("saveImage: skipping %q due to read error %v\n", job.InputPath, job.Err)
				out <- job
				continue
			}

			if job.OutPath == "" {
				fmt.Printf("saveImageL:empty output path for input %q\n", job.InputPath)
				out <- job
				continue
			}

			if job.Image == nil {
				fmt.Printf("saveImage: nil image for output path %q\n", job.OutPath)
				out <- job
				continue
			}

			t0 := time.Now()
			imageprocessing.WriteImage(job.OutPath, job.Image)
			job.Timings.Save = time.Since(t0)
			out <- job
		}
		close(out)
	}()
	return out
}

func runPipeline(imagePaths []string) BenchmarkResult {
	start := time.Now()

	channel1 := loadImage(imagePaths)
	channel2 := resize(channel1)
	channel3 := convertToGrayscale(channel2)
	writeResults := saveImage(channel3)

	result := BenchmarkResult{
		Mode:      "goroutine pipeline",
		TotalJobs: len(imagePaths),
		PerStage:  map[string]time.Duration{"load": 0, "resize": 0, "grayscale": 0, "save": 0},
	}

	for job := range writeResults {
		if job.Err != nil {
			result.Failed++
		} else {
			result.Succeeded++
			//fmt.Printf("  [pipeline] saved %s\n", job.OutPath)
		}
		result.PerStage["load"] += job.Timings.Load
		result.PerStage["resize"] += job.Timings.Resize
		result.PerStage["grayscale"] += job.Timings.Gray
		result.PerStage["save"] += job.Timings.Save
	}

	result.Elapsed = time.Since(start)
	result.Throughput = float64(result.TotalJobs) / result.Elapsed.Seconds()
	return result

}

func processImageSequential(path string) Job {
	job := Job{
		InputPath: path,
		OutPath:   strings.Replace(path, "images/", "images/output/", 1),
	}

	t0 := time.Now()
	if path == "" {
		job.Err = fmt.Errorf("image load failed: empty file path")
		job.Timings.Load = time.Since(t0)
		return job
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		job.Err = fmt.Errorf("image load failed: file does not exist %q", path)
		job.Timings.Load = time.Since(t0)
		return job
	}

	img := imageprocessing.ReadImage(path)
	job.Timings.Load = time.Since(t0)
	if img == nil {
		job.Err = fmt.Errorf("image load failed: ReadImage returned nil for %q", path)
		return job
	}
	job.Image = img

	t0 = time.Now()
	job.Image = imageprocessing.Resize(job.Image)
	job.Timings.Resize = time.Since(t0)

	t0 = time.Now()
	job.Image = imageprocessing.Grayscale(job.Image)
	job.Timings.Gray = time.Since(t0)

	if job.OutPath == "" {
		job.Err = fmt.Errorf("empty output path for input %q", path)
		return job
	}

	t0 = time.Now()
	imageprocessing.WriteImage(job.OutPath, job.Image)
	job.Timings.Save = time.Since(t0)

	return job
}

func runSequential(paths []string) BenchmarkResult {
	start := time.Now()

	result := BenchmarkResult{
		Mode:      "sequential",
		TotalJobs: len(paths),
		PerStage:  map[string]time.Duration{"load": 0, "resize": 0, "grayscale": 0, "save": 0},
	}

	for _, p := range paths {
		job := processImageSequential(p)
		if job.Err != nil {
			result.Failed++
			//fmt.Printf("  [sequential] error for %q: %v\n", p, job.Err)
		} else {
			result.Succeeded++
			//fmt.Printf("  [sequential] saved %s\n", job.OutPath)
		}
		result.PerStage["load"] += job.Timings.Load
		result.PerStage["resize"] += job.Timings.Resize
		result.PerStage["grayscale"] += job.Timings.Gray
		result.PerStage["save"] += job.Timings.Save
	}

	result.Elapsed = time.Since(start)
	result.Throughput = float64(result.TotalJobs) / result.Elapsed.Seconds()
	return result
}

type RunMode int

const (
	ModePipeline   RunMode = iota
	ModeSequential RunMode = iota
	ModeBoth       RunMode = iota
)

func averageResults(results []BenchmarkResult) BenchmarkResult {
	if len(results) == 0 {
		return BenchmarkResult{}
	}

	avg := BenchmarkResult{
		Mode:      results[0].Mode + " (avg)",
		TotalJobs: results[0].TotalJobs,
		PerStage:  map[string]time.Duration{"load": 0, "resize": 0, "grayscale": 0, "save": 0},
	}
	for _, r := range results {
		avg.Succeeded += r.Succeeded
		avg.Failed += r.Failed
		avg.Elapsed += r.Elapsed
		avg.Throughput += r.Throughput
		for k := range avg.PerStage {
			avg.PerStage[k] += r.PerStage[k]
		}
	}
	n := time.Duration(len(results))
	avg.Succeeded /= len(results)
	avg.Failed /= len(results)
	avg.Elapsed /= n
	avg.Throughput /= float64(len(results))
	for k := range avg.PerStage {
		avg.PerStage[k] /= n
	}
	return avg
}

func Benchmark(paths []string, mode RunMode, iterations int) {
	fmt.Printf("\nRunning benchmark: %d iteration(s), %d image(s) each\n",
		iterations, len(paths))

	runOnce := func(fn func([]string) BenchmarkResult, label string) {
		var runs []BenchmarkResult
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func(iter int) {
				defer wg.Done()
				//fmt.Printf("\n--- %s  iteration %d ---\n", label, iter+1)
				r := fn(paths)
				mu.Lock()
				runs = append(runs, r)
				mu.Unlock()
			}(i)
		}
		wg.Wait()

		if iterations > 1 {
			avg := averageResults(runs)
			avg.Print()
		} else {
			runs[0].Print()
		}
	}

	switch mode {
	case ModePipeline:
		runOnce(runPipeline, "pipeline")
	case ModeSequential:
		runOnce(runSequential, "sequential")
	case ModeBoth:
		runOnce(runPipeline, "pipeline")
		runOnce(runSequential, "sequential")
	}
}

func main() {

	imagePaths := []string{"images/image1.jpg",
		"images/image2.jpg",
		"images/image3.jpg",
		"images/image4.jpg",
		"images/image5.jpg",
	}

	Benchmark(imagePaths, ModeBoth, 100)

}
