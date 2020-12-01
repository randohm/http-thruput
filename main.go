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
    defaultChunkSize = 10485760
    defaultMode = "server"
    defaultGetSize = "1g"
    defaultPostSize = "1g"
    defaultRemoteHost = "localhost"
    defaultRemotePort = 8080
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
    chunkSize *int
    chunk []byte
    usageText = "[-h|-help] [-web.listen-address 0.0.0.0:8080] [-chunk.size 1000000]"
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
        //log.Printf("Sending %d bytes", numBytes)

        // Send bytes
        startTime := time.Now()
        for i := int64(0) ; i < numBytes ; i++ {
            if i < numBytes - int64(*chunkSize) {
                _, err = w.Write(chunk)
                if err != nil {
                    log.Print(err)
                    return
                }
                i += int64(*chunkSize)
            } else {
                bytesLeft := numBytes - i
                _, err := w.Write(chunk[:bytesLeft])
                if err != nil {
                    log.Print(err)
                    return
                }
                i = numBytes
            }
        }
        stopTime := time.Now()
        elapsed := stopTime.Sub(startTime)
        rate := float64(numBytes)/float64(elapsed.Milliseconds())*1000
        log.Printf("%s sent %s @ %s/sec in %.3fs", r.RemoteAddr, ppByteCount(float64(numBytes)), ppByteCount(rate), elapsed.Seconds())
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

    var buffer = make([]byte, *chunkSize)
    var bodySize int64 = 0
    startTime := time.Now()
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
    stopTime := time.Now()
    elapsed := stopTime.Sub(startTime)
    ulRate := float64(bodySize)/float64(elapsed.Milliseconds())*1000
    log.Printf("%s received %s @ %s/sec in %.3fs", r.RemoteAddr, ppByteCount(float64(bodySize)), ppByteCount(ulRate), elapsed.Seconds())
}



// Sets up the HTTP listener
func runServer(listenAddress string) {
    chunk = []byte(strings.Repeat("1", *chunkSize)) // Initialize dummy data
    http.HandleFunc(downloadPath, downloadRequestHandler)
    http.HandleFunc(uploadPath, uploadRequestHandler)

    log.Printf("Listening on %s", listenAddress)
    err := http.ListenAndServe(listenAddress, nil)
    if err != nil {
        log.Fatal(err)
    }
}



/*
    Calls the GET and POST tests. Manages sync for parallel execution.
    Args:
      remoteHost: hostname/IP of HTTP server
      remotePort: TCP port on HTTP server
      runGet: if false, don't run the get test
      getSize: size of data to use on the get test
      runPost: if false, don't run the post test
      postSize: size of data to use on the post tesl
      parallel: if true, run tests in parallel. Run in series if false.
*/
func runClient(remoteHost string, remotePort int, runGet bool, getSize string, runPost bool, postSize string, parallel bool) {
    // Generate URLs
    postUrl := fmt.Sprintf("http://%s:%d%s", remoteHost, remotePort, uploadPath)
    getUrl := fmt.Sprintf("http://%s:%d%s?s=%s", remoteHost, remotePort, downloadPath, getSize)

    // Run tests
    if parallel && runGet && runPost {
        var wg sync.WaitGroup
        wg.Add(1)
        go runPostTest(postUrl, postSize, &wg)
        wg.Add(1)
        go runGetTest(getUrl, &wg)
        wg.Wait()
    } else {
        if runGet {
            runGetTest(getUrl, nil)
        }
        if runPost {
            runPostTest(postUrl, postSize, nil)
        }
    }
}



// GETs from getUrl. Reads and discards the data.
func runGetTest(getUrl string, wg *sync.WaitGroup) {
    if wg != nil {
        defer wg.Done()
    }
    var buffer = make([]byte, *chunkSize)
    var bodySize int64 = 0

    // Make the GET call, read in and discard the body data
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
    defer res.Body.Close()
    if res.StatusCode != 200 {
        log.Printf("Returned HTTP status %s", res.Status)
        return
    }

    elapsed := stopTime.Sub(startTime)
    dlRate := float64(bodySize)/float64(elapsed.Milliseconds())*1000
    log.Printf("GET Rate: %s/sec Received: %s Elapsed: %.3fs", ppByteCount(dlRate), ppByteCount(float64(bodySize)), elapsed.Seconds())
}



// POSTs to postUrl with postSize amount of data.
// To reduce memory use, the io.Reader passed to http.Post() is
// fed data from a pipe and written to from a go routine.
func runPostTest(postUrl string, postSize string, wg *sync.WaitGroup) {
    if wg != nil {
        defer wg.Done()
    }

    pipeR, pipeW:= io.Pipe()    // Pipe to write post data
    var writtenBytes int64 = 0  // Track number of bytes written
    numBytes, err := getByteCount(postSize)
    if err != nil {
        log.Print(err)
        return
    }

    // Go routine for writing to the pipe
    go func() {
        dummyBlock := []byte(strings.Repeat("1", int(*chunkSize)))    // Allocate dummy data chunk
        for writtenBytes < numBytes {
            bytesLeft := numBytes - writtenBytes
            if bytesLeft < int64(*chunkSize) {
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

    // Make the POST call
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
    chunkSize = flag.Int("chunk.size", defaultChunkSize, "Chunk size for in-memory file chunks")
    mode := flag.String("mode", defaultMode, "Run in 'client' or 'server' mode.")
    testSize := flag.String("size", "", "Size of data. 1g, 2M, 4k, etc. Overrides -size.post and -size.get.")
    postSize := flag.String("size.post", defaultPostSize, "Size of post data. 1g, 2M, 4k, etc.")
    getSize := flag.String("size.get", defaultGetSize, "Size of get data. 1g, 2M, 4k, etc.")
    parallelRun := flag.Bool("parallel", defaultParallelRun, "Run client in parallel or series")
    runGet := flag.Bool("run.get", defaultRunGet, "Run GET test.")
    runPost := flag.Bool("run.post", defaultRunPost, "Run POST test.")
    remoteHost := flag.String("host", defaultRemoteHost, "Hostname of server running http-thruput")
    remotePort := flag.Int("port", defaultRemotePort, "Remote server port")
    flag.Parse()

    if *testSize != "" {
        *getSize = *testSize
        *postSize = *testSize
    }

    log.Printf("Using chunk size %d", *chunkSize)
    if *mode == "client" {
        runClient(*remoteHost, *remotePort, *runGet, *getSize, *runPost, *postSize, *parallelRun)
    } else if *mode == "server" {
        runServer(*listenAddress)
    } else {
        fmt.Fprintf(os.Stderr, "Invalid value for -mode\n")
        os.Exit(1)
    }
}
