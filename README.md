code for processing images in go. Reads, resizes, grayscales, and saves image files.

Base code is from codeHeim's Golang tutorial, episode 21  
  https://www.youtube.com/watch?v=8Rn8yOQH62k  
  https://github.com/code-heim/go_21_goroutines_pipeline  

All files are the same as in the original code-heim repo besides main.go, the images folder, parallel_sequential_comparison.txt, and main_test.go

Changes to main.go  
  Modified the Job struct to track errors and runtimes  
  added StageTimings struct to keep track of runtimes for each stage in the job  
  added BenchmarkResult struct to keep track of the benchmark statistics  
  modified the loadImage and saveImage functions to include error checking  
  modified the loadImage, resize, convertToGrayscale, and saveImage functions to track and record runtimes  
  added a new function, runPipeline, to run the image processing with goroutines, while keeping track of runtimes and benchmarks  
  added new functions, processImageSequential and runSequential, that run the same image processing, but sequentially instead of using goroutines  
  added function averageResults to calculate the average runtime results over n iterations for comparing the goroutines and sequential processes  
  added the function Benchmark to run the selected RunMode, and process the results  

  Unit testing added with main_test.go

  run as:  
    go run main.go
  build as:
    go build main.go
  test as
    go test -v

  parallel_sequential_comparison.txt contains the average benchmarks of the goroutine and sequential processes run 100 times on my computer. While the average runtimes of the individual parts is lower in the sequential code, the wall time is lower when using goroutine. This makes sense, as goroutine needs to split its resources over multiple processes, but is able to more efficiently use those resources through the whole runtime. Goroutine could also be more helpful if the pipeline wasn't strictly linear in the first place. For example, if instead of changing an image to greyscale, it made three new images using only hues of red, blue, and green, which could each immediately be done as soon as the resize step finished.

  I used claude.ai to ask specific syntax questions, and to make the unit tests.
