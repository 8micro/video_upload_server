// An upload server for fineuploader.com javascript upload library
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var port = flag.Int("p", 8080, "Port number to listen to, defaults to 8080")
var uploadDir = flag.String("d", "uploads", "Upload directory, defaults to 'uploads'")

var ffprobePath = flag.String("ffprobe", "/data/web/ffmpeg_ubuntu/ffmpeg-git-20171206-64bit-static/ffprobe", "ffprobe path name")
var ffmpegPath = flag.String("ffmpeg", "", "ffmpeg path name")

// Request parameters
const (
	paramUuid   = "qquuid" // uuid
	paramFile   = "qqfile" // file name
	paramUserId = "userid" // userid
)

// Chunked request parameters
const (
	paramPartIndex       = "qqpartindex"      // part index
	paramPartBytesOffset = "qqpartbyteoffset" // part byte offset
	paramTotalFileSize   = "qqtotalfilesize"  // total file size
	paramTotalParts      = "qqtotalparts"     // total parts
	paramFileName        = "qqfilename"       // file name for chunked requests
	paramChunkSize       = "qqchunksize"      // size of the chunks

)

type UploadResponse struct {
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	PreventRetry bool   `json:"preventRetry"`
}

type VideoInfo struct {
	VideoWidth  string
	VideoHeight string
	Rate        string
	Duration    string
	Size        string
}

//must have userid and prefix for saving uploading files
func main() {
	flag.Parse()
	hostPort := fmt.Sprintf("0.0.0.0:%d", *port)
	log.Printf("Initiating server listening at [%s]", hostPort)
	log.Printf("Base upload directory set to [%s]", *uploadDir)
	http.HandleFunc("/upload", UploadHandler)
	http.HandleFunc("/chunksdone", ChunksDoneHandler)
	http.Handle("/upload/", http.StripPrefix("/upload/", http.HandlerFunc(UploadHandler)))
	log.Fatal(http.ListenAndServe(hostPort, nil))
}

func UploadHandler(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		upload(w, req)
		return
	case http.MethodDelete:
		//delete(w, req)
		return
	}
	errorMsg := fmt.Sprintf("Method [%s] is not supported:", req.Method)
	http.Error(w, errorMsg, http.StatusMethodNotAllowed)
}

//return pathname path
func GetFilePathNameAndPath(userId string, uuid string, filename string) (string, string) {
	//get yyyymmddhhmmss
	year := time.Now().Year()
	month := time.Now().Month()
	day := time.Now().Day()
	hour := time.Now().Hour()
	min := time.Now().Minute()
	second := time.Now().Second()

	recvTime := fmt.Sprintf("%d-%d-%d-%d-%d-%d", year, month, day, hour, min, second)

	// +userid+yyyymmdd+origname+.mp4
	//upload/138483/9a3c83eb-2560-42df-b9d7-901b54b5161f/138483-20180101-testvideo.mp4
	filePathName := fmt.Sprintf("%s/%s/%s/%s-%s-%s-%s", *uploadDir, userId, uuid, uuid, userId, recvTime, filename)
	filePath := fmt.Sprintf("%s/%s/%s/", *uploadDir, userId, uuid)
	log.Println("file path: %v filepathname %v", filePath, filePathName)
	return filePathName, filePath
}

func upload(w http.ResponseWriter, req *http.Request) {
	uuid := req.FormValue(paramUuid)
	if len(uuid) == 0 {
		log.Printf("No uuid received, invalid upload request")
		http.Error(w, "No uuid received", http.StatusBadRequest)
		return
	}
	log.Printf("Starting upload handling of request with uuid of [%s]\n", uuid)
	file, headers, err := req.FormFile(paramFile)
	if err != nil {
		writeUploadResponse(w, err)
		return
	}
	log.Printf("file headers : %v \n", headers)
	userId := req.FormValue("userid")
	log.Printf("upload userid is %v", userId)

	var filename string
	filename = headers.Filename

	partIndex := req.FormValue(paramPartIndex)
	log.Printf("part index is %v filename is %v \n", partIndex, filename)

	filePathName, fileDir := GetFilePathNameAndPath(userId, uuid, filename)
	if err := os.MkdirAll(fileDir, 0777); err != nil {
		writeUploadResponse(w, err)
		return
	}

	outfile, err := os.Create(filePathName)
	if err != nil {
		writeUploadResponse(w, err)
		return
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, file)
	if err != nil {
		writeUploadResponse(w, err)
		return
	}

	writeUploadResponse(w, nil)
	log.Printf("upload file %s  to dir %s done ", filename, filePathName)

	GetVideoBasicInfo(filePathName, userId, uuid)
	//executeFfprobeCommand()
}

func delete(w http.ResponseWriter, req *http.Request) {
	log.Printf("Delete request received for uuid [%s]", req.URL.Path)
	err := os.RemoveAll(*uploadDir + "/" + req.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)

}

func parseFfprobeResult(result []byte) (*VideoInfo, error) {
	return nil, errors.New("test")
}

func GetVideoBasicInfo(filePathname string, userId string, guid string) {
	fullPathName := filePathname
	args := fmt.Sprintf("  -v error -show_format -show_streams -print_format flat %s", fullPathName)

	scriptFilePath := fmt.Sprintf("/tmp/videoscripts/%s/", userId)
	err := os.MkdirAll(scriptFilePath, 0777)
	if err != nil {
		log.Println("create script dir failed %v", scriptFilePath)
		return
	}
	scriptFilePathname := fmt.Sprintf("/tmp/%s.sh", userId)
	f, err0 := os.Create(scriptFilePathname)
	if err0 != nil {
		log.Printf("create script file error")
		return
	}
	scriptHeader := "#!/bin/bash\n"

	_, err1 := f.Write([]byte(scriptHeader))
	if err1 != nil {
		log.Printf("write script header failed")
		return
	}
	scripts := *ffprobePath + args

	_, err2 := f.Write([]byte(scripts))
	f.Close()

	err3 := os.Chmod(scriptFilePathname, 777)
	if err3 != nil {
		log.Printf("change script privellege failed")
		return
	}

	cmd := exec.Command(scriptFilePathname)
	data, err2 := cmd.Output()
	if err2 != nil {
		log.Printf("execute output is  %v", string(data[:]))
	}
	ParseVideoInfoStr(string(data[:]))
}

func ParseVideoInfoStr(strData string) {
	strLines := strings.Split(strData, "\n")
	var videoInfo VideoInfo

	for _, ele := range strLines {

		if strings.Contains(ele, ".width=") {
			ele_value := strings.Split(ele, "=")
			if len(ele_value) == 2 {
				videoInfo.VideoWidth = strings.Trim(ele_value[1], " ")
			} else {
				log.Printf("read video width failed %v", ele)
			}
		}

		if strings.Contains(ele, ".height=") {
			ele_value := strings.Split(ele, "=")
			if len(ele_value) == 2 {
				videoInfo.VideoHeight = strings.Trim(ele_value[1], " ")
			} else {
				log.Printf("read video height failed %v", ele)
			}
		}

		if strings.Contains(ele, ".duration=") {
			ele_value := strings.Split(ele, "=")
			if len(ele_value) == 2 {
				videoInfo.Duration = strings.Trim(ele_value[1], " ")
			} else {
				log.Printf("read video duration failed %v", ele)
			}
		}

		if strings.Contains(ele, "format.bit_rate=") {
			ele_value := strings.Split(ele, "=")
			if len(ele_value) == 2 {
				videoInfo.Rate = strings.Trim(ele_value[1], " ")
			} else {
				log.Printf("read video bit rate failed %v", ele)
			}
		}

		if strings.Contains(ele, "format.size=") {
			ele_value := strings.Split(ele, "=")
			if len(ele_value) == 2 {
				videoInfo.Size = strings.Trim(ele_value[1], " ")
			} else {
				log.Printf("read video bit rate failed %v", ele)
			}
		}

	}

	log.Printf("analyze video info %v", videoInfo)

	//todo
	//1. call api to tell the videoinfo to db server
	//2. call api to tell split server to split it into splits
}

func executeFfprobeCommand(filename string, userId string, guid string) ([]byte, error) {
	if len(*ffprobePath) == 0 {
		log.Printf("ffprobe path is empty")
		return nil, fmt.Errorf("parameter error")
	}

	filePathName := userId + "/" + guid + "/" + filename

	args := fmt.Sprintf(" -v error -show_format -show_streams -print_format flat %s", filePathName)

	//ffprobe -v error -show_format -show_streams -print_format flat  test.mp4

	cmd := exec.Command(*ffprobePath, args)
	err := cmd.Run()
	//data, err2 = cmd.Output()
	if err != nil {
		log.Printf("execute command ffprobe %s  failed ", args)
		return nil, fmt.Errorf("execute command ffprobe %s  failed", args)
	}

	return nil, nil
}

func ChunksDoneHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("ChunksDoneHandler")

	if req.Method != http.MethodPost {
		errorMsg := fmt.Sprintf("Method [%s] is not supported", req.Method)
		http.Error(w, errorMsg, http.StatusMethodNotAllowed)
	}
	uuid := req.FormValue(paramUuid)
	filename := req.FormValue(paramFileName)
	req.FormValue(paramUserId)
	totalFileSize, err := strconv.Atoi(req.FormValue(paramTotalFileSize))
	if err != nil {
		writeHttpResponse(w, http.StatusInternalServerError, err)
		return
	}
	totalParts, err := strconv.Atoi(req.FormValue(paramTotalParts))
	if err != nil {
		writeHttpResponse(w, http.StatusInternalServerError, err)
		return
	}

	finalFilename := fmt.Sprintf("%s/%s/%s", *uploadDir, uuid, filename)
	f, err := os.Create(finalFilename)
	if err != nil {
		writeHttpResponse(w, http.StatusInternalServerError, err)
		return
	}
	defer f.Close()

	var totalWritten int64
	for i := 0; i < totalParts; i++ {
		part := fmt.Sprintf("%[1]s/%[2]s/%[2]s_%05[3]d", *uploadDir, uuid, i)
		partFile, err := os.Open(part)
		if err != nil {
			writeHttpResponse(w, http.StatusInternalServerError, err)
			return
		}
		written, err := io.Copy(f, partFile)
		if err != nil {
			writeHttpResponse(w, http.StatusInternalServerError, err)
			return
		}
		partFile.Close()
		totalWritten += written

		if err := os.Remove(part); err != nil {
			log.Printf("Error: %v", err)
		}
	}

	if totalWritten != int64(totalFileSize) {
		errorMsg := fmt.Sprintf("Total file size mistmatch, expected %d bytes but actual is %d", totalFileSize, totalWritten)
		http.Error(w, errorMsg, http.StatusMethodNotAllowed)
	} else {
		// data, err := executeFfprobeCommand(filename, userid, uuid)
		// if err != nil {
		// 	log.Printf("execute ffprobe command failed")
		// 	return
		// } else {

		// }
	}
}

func writeHttpResponse(w http.ResponseWriter, httpCode int, err error) {
	w.WriteHeader(httpCode)
	if err != nil {
		log.Printf("An error happened: %v", err)
		w.Write([]byte(err.Error()))
	}
}

func writeUploadResponse(w http.ResponseWriter, err error) {
	uploadResponse := new(UploadResponse)
	if err != nil {
		uploadResponse.Error = err.Error()
	} else {
		uploadResponse.Success = true
	}
	w.Header().Set("Content-Type", "text/plain")
	json.NewEncoder(w).Encode(uploadResponse)

	log.Printf("upload file done!!")
}
