package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	logFilePath = "/home/ubuntu/bootstrap_logs"
	serverPort  = ":3333"
)

func main() {
	http.HandleFunc("/logs/stream", streamLogs)

	log.Printf("Starting log streaming server on %s", serverPort)
	log.Printf("Endpoint: http://localhost%s/logs/stream", serverPort)

	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func streamLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	file, err := os.Open(logFilePath)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	file.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(file)

	fmt.Fprintf(w, "event: connected\ndata: Streaming logs from %s\n\n", logFilePath)
	flusher.Flush()

	lastSize := info.Size()

	for {
		select {
		case <-r.Context().Done():
			log.Println("Client disconnected")
			return
		default:
			line, err := reader.ReadString('\n')

			if err != nil {
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)

					currentInfo, statErr := file.Stat()
					if statErr == nil && currentInfo.Size() < lastSize {
						log.Println("Log file was truncated, resetting position")
						file.Seek(0, io.SeekStart)
						reader = bufio.NewReader(file)
						lastSize = 0
					} else if statErr == nil {
						lastSize = currentInfo.Size()
					}
					continue
				}
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				flusher.Flush()
				return
			}
			if strings.TrimSpace(line) == "__BOOTSTRAP_DONE__" {
				log.Println("Bootstrap complete sentinel seen, stopping stream")
				return
			}

			fmt.Fprintf(w, "data: %s\n", line)
			flusher.Flush()
		}
	}
}
