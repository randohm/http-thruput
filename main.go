package main

import (
    "flag"
    "net/http"
    "log"
    "regexp"
    "fmt"
    "strings"
    "os"
    "path"
    "time"
    "sync"
    "io"
    "errors"
)

const (
    defaultListenAddress = "0.0.0.0:8080"
    defaultSegmentSize = 10485760
    defaultMode = "server"
    defaultPostUrl = "http://localhost:8080"+uploadPath
    defaultPostSize = "1g"
    defaultGetUrl = "http://localhost:8080"+downloadPath+"?s=1g"
    defaultGetSize = "1g"
    defaultHost = "localhost"
    defaultHostPost = 8080
    defaultParallelRun = true
    defaultRunPost = true
    defaultRunGet = true

    downloadPath = "/down"
    uploadPath = "/up"
    uploadContentType = "application/octet-stream"
    byteStringRegex = "^([0-9]+)([bBkKmMgG]?)$"
)

var (
    multiplier = map[string]int64{
        "": 1,
        "b": 1,
        "B": 1,
        "k": 1024,
        "K": 1000,
        "m": 1048576,
        "M": 1000000,
        "g": 1073741824,
        "G": 1000000000,
    }
    segmentSize *int
    segment []byte
    usageText = "[-h|-help] [-web.listen-address 0.0.0.0:8080] [-segment.size 1000000]"
    helpText = "Small webserver for bandwidth testing.\n"+
        "Accepts parameter `s` with a value for the number of bytes\n"+
        "Example values of `s`: 1g, 20M, 123412\n\n"+
        "Options:"
)



// Handles the download test.
// Based on the 's' parameter, outputs the number of bytes specified.
func downloadRequestHandler(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query()
    if query["s"] != nil {
        // Validate input. Make sure it is only a string of numbers with an optional 1-char suffix
        matched, err := regexp.MatchString(byteStringRegex, query["s"][0])
        if err != nil {
            http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
            log.Print(err)
            return
        }
        // Return 400 for invalid inputes
        if !matched {
            http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
            log.Printf("Incorrect request format")
            return
        }

        numBytes, err := getByteCount(query["s"][0])
        if err != nil {
            http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
            log.Print(err)
            return
        }
        log.Printf("Sending %d bytes", numBytes)

        // Send bytes
        for i := int64(0) ; i < numBytes ; i++ {
            if i < numBytes - int64(*segmentSize) {
                w.Write(segment)
                i += int64(*segmentSize)
            } else {
                bytesLeft := numBytes - i
                w.Write(segment[:bytesLeft])
                i = numBytes
            }
        }
    }
}



// Convert strings like 1g, 4m to integers of the number of bytes
// Only integers are supported.
// Returns int64 with the number of bytes
func getByteCount(bytes string) (int64, error) {
    // Validate input
    matched, err := regexp.MatchString(byteStringRegex, bytes)
    if err != nil {
        log.Print(err)
        return -1, err
    }
    if !matched {
        return -1, errors.New("Incorrect request format")
    }

    var count int64
    var unit string
    fmt.Sscan(bytes, &count, &unit)
    return count*multiplier[unit], nil
}



// Returns a string like '64MiB', '3.2KiB' based on numBytes
// numBytes is type float64 to accomodate for rates, not just byte counts
func ppByteCount(numBytes float64) string {
    switch {
        case numBytes > float64(multiplier["g"]):
            return fmt.Sprintf("%.2f GiB", numBytes/float64(multiplier["g"]))
        case numBytes > float64(multiplier["m"]):
            return fmt.Sprintf("%.2f MiB", numBytes/float64(multiplier["m"]))
        case numBytes > float64(multiplier["k"]):
            return fmt.Sprintf("%.2f KiB", numBytes/float64(multiplier["k"]))
    }
    return fmt.Sprintf("%.2f B", numBytes)
}



// Handler for upload tests. Accepts only POST.
// Reads and discards data from the request body
func uploadRequestHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
        return
    }
    err := r.ParseForm()
    if err != nil {
        http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
        log.Print(err)
        return
    }

    var buffer = make([]byte, *segmentSize)
    var bodySize int64 = 0
    for {
        bytesRead, err := r.Body.Read(buffer)
        bodySize += int64(bytesRead)
        if err != nil {
            if err == io.EOF {
                break
            }
            http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
            log.Print(err)
            return
        }
    }
    log.Printf("Received %d bytes\n", bodySize)
}



// Sets up the HTTP listener
func runServer(listenAddress string) {
    segment = []byte(strings.Repeat("1", *segmentSize)) // Initialize dummy data
    http.HandleFunc(downloadPath, downloadRequestHandler)
    http.HandleFunc(uploadPath, uploadRequestHandler)

    log.Printf("Listening on %s", listenAddress)
    http.ListenAndServe(listenAddress, nil)
}



// Sets up the GET and POST tests.
// 'parallel' determines if the tests are run in series or parallel
func runClient(runGet bool, getUrl string, getSize string, runPost bool, postUrl string, postSize string, parallel bool) {
    var wg sync.WaitGroup
    if parallel && runGet && runPost {
        wg.Add(1)
        go runPostTest(&wg, postUrl, postSize)
        wg.Add(1)
        go runGetTest(&wg, getUrl)
    } else {
        if runGet {
            wg.Add(1)
            runGetTest(&wg, getUrl)
        }
        if runPost {
            wg.Add(1)
            runPostTest(&wg, postUrl, postSize)
        }
    }
    wg.Wait()
}



// GETs from getUrl. Reads and discards the data.
func runGetTest(wg *sync.WaitGroup, getUrl string) {
    defer wg.Done()
    var buffer = make([]byte, *segmentSize)
    var bodySize int64 = 0

    log.Printf("Getting %s", getUrl)
    startTime := time.Now()
    res, err := http.Get(getUrl)
    if err != nil {
        log.Print(err)
        return
    }
    for {
        bytesRead, err := res.Body.Read(buffer)
        bodySize += int64(bytesRead)
        if err != nil {
            if err == io.EOF {
                break
            }
            log.Print(err)
            return
        }
    }
    stopTime := time.Now()
    elapsed := stopTime.Sub(startTime)
    defer res.Body.Close()
    if res.StatusCode != 200 {
        log.Printf("Returned HTTP status %s", res.Status)
        return
    }

    dlRate := float64(bodySize)/float64(elapsed.Milliseconds())*1000
    log.Printf("GET Rate: %s/sec Received: %s Elapsed: %.3fs", ppByteCount(dlRate), ppByteCount(float64(bodySize)), elapsed.Seconds())
}



// POSTs to postUrl with postSize amount of data.
// To reduce memory use, the io.Reader passed to http.Post() is
// fed data from a pipe and written to from a go routine.
func runPostTest(wg *sync.WaitGroup, postUrl string, postSize string) {
    defer wg.Done()
    numBytes, err := getByteCount(postSize)
    if err != nil {
        log.Print(err)
        return
    }

    pipeR, pipeW:= io.Pipe() // Pipe to write post data
    var writtenBytes int64 = 0  // Track number of bytes written

    // Go routine for writing to the pipe
    go func() {
        dummyBlock := []byte(strings.Repeat("1", int(*segmentSize)))    // Allocate dummy data chunk
        for writtenBytes < numBytes {
            bytesLeft := numBytes - writtenBytes
            if bytesLeft < int64(*segmentSize) {
                // This will be the last chunk, fix the size
                dummyBlock = dummyBlock[:bytesLeft]
            }
            b, err := pipeW.Write(dummyBlock)
            writtenBytes += int64(b)
            if err != nil {
                log.Print(err)
                return
            }
        }
        pipeW.Close()
    }()

    log.Printf("Posting %d bytes to '%s'", numBytes, postUrl)
    startTime := time.Now()
    res, err := http.Post(postUrl, uploadContentType, pipeR)
    stopTime := time.Now()
    if err != nil {
        log.Print(err)
        return
    }
    if res.StatusCode != 200 {
        log.Printf("Returned HTTP status %s", res.Status)
        return
    }

    elapsed := stopTime.Sub(startTime)
    ulRate := float64(numBytes)/float64(elapsed.Milliseconds())*1000
    log.Printf("POST Rate: %s/sec Sent: %s Elapsed: %.3fs", ppByteCount(ulRate), ppByteCount(float64(writtenBytes)), elapsed.Seconds())
}



func init() {
    log.SetFlags(log.Ldate|log.Ltime|log.Lshortfile)
    flag.Usage = func() {
        fmt.Fprintf(os.Stderr, "Usage: %s %s\n%s\n", path.Base(os.Args[0]), usageText, helpText)
        flag.PrintDefaults()
    }
}



func main() {
    listenAddress := flag.String("web.listen-address", defaultListenAddress, "Listen address for HTTP requests")
    segmentSize = flag.Int("segment.size", defaultSegmentSize, "Listen address for HTTP requests")
    mode := flag.String("mode", defaultMode, "'client' or 'server'")
    postUrl := flag.String("url.post", defaultPostUrl, "URL to post data for upload test")
    postSize := flag.String("size.post", defaultPostSize, "Size of post data. 1g, 2M, 4k, etc.")
    getUrl := flag.String("url.get", defaultGetUrl, "URL to get data for download test")
    getSize := flag.String("size.get", defaultGetSize, "Size of get data. 1g, 2M, 4k, etc.")
    parallelRun := flag.Bool("parallel", defaultParallelRun, "Run client in parallel or series")
    runGet := flag.Bool("run.get", defaultRunGet, "Run GET test. False is only effective with -parallel=0")
    runPost := flag.Bool("run.post", defaultRunPost, "Run POST test. False is only effective with -parallel=0")
    flag.Parse()

    log.Printf("Using segment size %d", *segmentSize)
    if *mode == "client" {
        runClient(*runGet, *getUrl, *getSize, *runPost, *postUrl, *postSize, *parallelRun)
    } else if *mode == "server" {
        runServer(*listenAddress)
    } else {
        fmt.Fprintf(os.Stderr, "Invalid value for -mode\n")
        os.Exit(1)
    }
}
