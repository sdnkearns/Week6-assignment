package main

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"
)

// --- helpers ---

// newRGBA returns a trivial 10×10 solid-color image.
func newRGBA(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// makeJobs pumps a slice of Job values into a channel and closes it.
func makeJobs(jobs []Job) <-chan Job {
	ch := make(chan Job, len(jobs))
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	return ch
}

// drain collects every Job emitted by a channel.
func drain(ch <-chan Job) []Job {
	var out []Job
	for j := range ch {
		out = append(out, j)
	}
	return out
}

// --- StageTimings ---

func TestStageTimings_ZeroValue(t *testing.T) {
	var st StageTimings
	if st.Load != 0 || st.Resize != 0 || st.Gray != 0 || st.Save != 0 {
		t.Error("zero-value StageTimings should have all durations == 0")
	}
}

// --- Job ---

func TestJob_DefaultFields(t *testing.T) {
	j := Job{InputPath: "a.jpg"}
	if j.Err != nil {
		t.Error("new Job should have nil Err")
	}
	if j.Image != nil {
		t.Error("new Job should have nil Image")
	}
}

// --- BenchmarkResult.Print (smoke test – must not panic) ---

func TestBenchmarkResult_Print_DoesNotPanic(t *testing.T) {
	r := BenchmarkResult{
		Mode:       "test",
		TotalJobs:  3,
		Succeeded:  2,
		Failed:     1,
		Elapsed:    42 * time.Millisecond,
		Throughput: 47.6,
		PerStage: map[string]time.Duration{
			"load":      1 * time.Millisecond,
			"resize":    2 * time.Millisecond,
			"grayscale": 3 * time.Millisecond,
			"save":      4 * time.Millisecond,
		},
	}
	// should not panic
	r.Print()
}

// --- resize stage ---

func TestResize_SkipsErrJobs(t *testing.T) {
	sentinel := image.NewRGBA(image.Rect(0, 0, 5, 5))
	in := makeJobs([]Job{
		{InputPath: "bad.jpg", Err: fmt.Errorf("load failed"), Image: sentinel},
	})

	out := drain(resize(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	// Image must be unchanged when Err is set.
	if out[0].Image != sentinel {
		t.Error("resize must not mutate Image when job carries an error")
	}
	if out[0].Timings.Resize != 0 {
		t.Error("resize must not record timing when job carries an error")
	}
}

func TestResize_PassesJobThrough(t *testing.T) {
	// Use a real image so the resize helper can operate on it.
	img := newRGBA(color.RGBA{255, 0, 0, 255})
	in := makeJobs([]Job{{InputPath: "ok.jpg", Image: img}})

	out := drain(resize(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	if out[0].Err != nil {
		t.Errorf("unexpected error after resize: %v", out[0].Err)
	}
	if out[0].Image == nil {
		t.Error("Image must not be nil after resize")
	}
}

func TestResize_TimingRecorded(t *testing.T) {
	img := newRGBA(color.RGBA{0, 255, 0, 255})
	in := makeJobs([]Job{{InputPath: "ok.jpg", Image: img}})

	out := drain(resize(in))

	// Resize timing may be zero on very fast machines, but must be non-negative.
	if out[0].Timings.Resize < 0 {
		t.Error("resize timing must be non-negative")
	}
}

// --- convertToGrayscale stage ---

func TestConvertToGrayscale_SkipsErrJobs(t *testing.T) {
	sentinel := newRGBA(color.RGBA{0, 0, 255, 255})
	in := makeJobs([]Job{
		{InputPath: "bad.jpg", Err: fmt.Errorf("earlier stage failed"), Image: sentinel},
	})

	out := drain(convertToGrayscale(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	if out[0].Image != sentinel {
		t.Error("convertToGrayscale must not mutate Image when job carries an error")
	}
	if out[0].Timings.Gray != 0 {
		t.Error("convertToGrayscale must not record timing when job carries an error")
	}
}

func TestConvertToGrayscale_TimingRecorded(t *testing.T) {
	img := newRGBA(color.RGBA{128, 64, 32, 255})
	in := makeJobs([]Job{{InputPath: "ok.jpg", Image: img}})

	out := drain(convertToGrayscale(in))

	if out[0].Timings.Gray < 0 {
		t.Error("grayscale timing must be non-negative")
	}
	if out[0].Image == nil {
		t.Error("Image must not be nil after grayscale conversion")
	}
}

// --- saveImage stage ---

func TestSaveImage_SkipsJobsWithErr(t *testing.T) {
	in := makeJobs([]Job{
		{InputPath: "bad.jpg", Err: fmt.Errorf("load failed")},
	})

	out := drain(saveImage(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	// Error must be preserved, not overwritten.
	if out[0].Err == nil {
		t.Error("saveImage must preserve the existing error")
	}
}

func TestSaveImage_SkipsNilImage(t *testing.T) {
	in := makeJobs([]Job{
		{InputPath: "ok.jpg", OutPath: "images/output/ok.jpg", Image: nil},
	})

	out := drain(saveImage(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	// Should have passed through without crashing or recording a save timing.
	if out[0].Timings.Save != 0 {
		t.Error("saveImage must not record timing for nil image")
	}
}

func TestSaveImage_EmptyOutPath(t *testing.T) {
	img := newRGBA(color.White)
	in := makeJobs([]Job{
		{InputPath: "ok.jpg", OutPath: "", Image: img},
	})

	out := drain(saveImage(in))

	if len(out) != 1 {
		t.Fatalf("want 1 job, got %d", len(out))
	}
	if out[0].Timings.Save != 0 {
		t.Error("saveImage must not record timing for empty OutPath")
	}
}

// --- averageResults ---

func TestAverageResults_Empty(t *testing.T) {
	avg := averageResults(nil)
	if avg.TotalJobs != 0 {
		t.Error("averageResults of empty slice should return zero BenchmarkResult")
	}
}

func TestAverageResults_Single(t *testing.T) {
	r := BenchmarkResult{
		Mode:       "sequential",
		TotalJobs:  5,
		Succeeded:  4,
		Failed:     1,
		Elapsed:    100 * time.Millisecond,
		Throughput: 40,
		PerStage: map[string]time.Duration{
			"load": 10 * time.Millisecond, "resize": 20 * time.Millisecond,
			"grayscale": 30 * time.Millisecond, "save": 40 * time.Millisecond,
		},
	}

	avg := averageResults([]BenchmarkResult{r})

	if avg.Succeeded != 4 {
		t.Errorf("want Succeeded=4, got %d", avg.Succeeded)
	}
	if avg.Elapsed != 100*time.Millisecond {
		t.Errorf("want Elapsed=100ms, got %v", avg.Elapsed)
	}
}

func TestAverageResults_Multiple(t *testing.T) {
	make := func(elapsed time.Duration, tp float64, succ, fail int) BenchmarkResult {
		return BenchmarkResult{
			Mode: "sequential", TotalJobs: 4,
			Succeeded: succ, Failed: fail,
			Elapsed: elapsed, Throughput: tp,
			PerStage: map[string]time.Duration{
				"load": elapsed, "resize": elapsed,
				"grayscale": elapsed, "save": elapsed,
			},
		}
	}

	results := []BenchmarkResult{
		make(100*time.Millisecond, 10, 3, 1),
		make(200*time.Millisecond, 20, 3, 1),
	}

	avg := averageResults(results)

	wantElapsed := 150 * time.Millisecond
	if avg.Elapsed != wantElapsed {
		t.Errorf("want avg Elapsed=%v, got %v", wantElapsed, avg.Elapsed)
	}
	if avg.Throughput != 15.0 {
		t.Errorf("want avg Throughput=15, got %f", avg.Throughput)
	}
	for _, stage := range []string{"load", "resize", "grayscale", "save"} {
		if avg.PerStage[stage] != wantElapsed {
			t.Errorf("stage %q: want %v, got %v", stage, wantElapsed, avg.PerStage[stage])
		}
	}
}

func TestAverageResults_ModeLabel(t *testing.T) {
	r := BenchmarkResult{
		Mode:     "pipeline",
		PerStage: map[string]time.Duration{"load": 0, "resize": 0, "grayscale": 0, "save": 0},
	}
	avg := averageResults([]BenchmarkResult{r, r})
	want := "pipeline (avg)"
	if avg.Mode != want {
		t.Errorf("want mode label %q, got %q", want, avg.Mode)
	}
}

// --- loadImage ---

func TestLoadImage_EmptyPath(t *testing.T) {
	ch := loadImage([]string{""})
	jobs := drain(ch)

	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	if jobs[0].Err == nil {
		t.Error("empty path must produce a non-nil Err")
	}
}

func TestLoadImage_NonExistentFile(t *testing.T) {
	ch := loadImage([]string{"does/not/exist.jpg"})
	jobs := drain(ch)

	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	// ReadImage returns nil for missing files → Err set.
	if jobs[0].Err == nil {
		t.Error("missing file must produce a non-nil Err")
	}
}

func TestLoadImage_OutPathDerived(t *testing.T) {
	// Even for a failing job the OutPath should be derived from InputPath.
	ch := loadImage([]string{"images/foo.jpg"})
	jobs := drain(ch)

	want := "images/output/foo.jpg"
	if jobs[0].OutPath != want {
		t.Errorf("want OutPath=%q, got %q", want, jobs[0].OutPath)
	}
}

func TestLoadImage_MultiplePathsAllEmitted(t *testing.T) {
	paths := []string{"", "", ""}
	ch := loadImage(paths)
	jobs := drain(ch)

	if len(jobs) != len(paths) {
		t.Errorf("want %d jobs, got %d", len(paths), len(jobs))
	}
}

// --- processImageSequential ---

func TestProcessImageSequential_EmptyPath(t *testing.T) {
	job := processImageSequential("")
	if job.Err == nil {
		t.Error("empty path must return a non-nil Err")
	}
}

func TestProcessImageSequential_MissingFile(t *testing.T) {
	job := processImageSequential("no/such/file.jpg")
	if job.Err == nil {
		t.Error("missing file must return a non-nil Err")
	}
}

func TestProcessImageSequential_OutPathSet(t *testing.T) {
	job := processImageSequential("images/sample.jpg")
	want := "images/output/sample.jpg"
	if job.OutPath != want {
		t.Errorf("want OutPath=%q, got %q", want, job.OutPath)
	}
}

// --- Benchmark concurrency safety ---

// Ensures Benchmark does not race when iterations > 1.
func TestBenchmark_ConcurrencyNoPanic(t *testing.T) {
	// Use empty-string paths so no real I/O occurs; all jobs will fail gracefully.
	paths := []string{"", ""}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Benchmark panicked: %v", r)
		}
	}()
	Benchmark(paths, ModeBoth, 3)
}

// Verifies the mutex in Benchmark actually protects `runs`.
func TestBenchmark_RunsAccumulatedSafely(t *testing.T) {
	var mu sync.Mutex
	var runs []BenchmarkResult

	const iterations = 20
	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := runSequential([]string{""})
			mu.Lock()
			runs = append(runs, r)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(runs) != iterations {
		t.Errorf("want %d results, got %d", iterations, len(runs))
	}
}
