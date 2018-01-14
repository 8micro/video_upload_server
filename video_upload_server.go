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
)

var port = flag.Int("p", 8080, "Port number to listen to, defaults to 8080")
var uploadDir = flag.String("d", "uploads", "Upload directory, defaults to 'uploads'")

var ffprobePath = flag.String("ffprobe", "", "ffprobe path name")
var ffmpegPath = flag.String("ffmpeg", "", "ffmpeg path name")

// Request parameters
const (
	paramUuid = "qquuid" // uuid
	paramFile = "qqfile" // file name
)

// Chunked request parameters
const (
	paramPartIndex       = "qqpartindex"      // part index
	paramPartBytesOffset = "qqpartbyteoffset" // part byte offset
	paramTotalFileSize   = "qqtotalfilesize"  // total file size
	paramTotalParts      = "qqtotalparts"     // total parts
	paramFileName        = "qqfilename"       // file name for chunked requests
	paramChunkSize       = "qqchunksize"      // size of the chunks
	paramUserId          = "userid"
)

type UploadResponse struct {
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	PreventRetry bool   `json:"preventRetry"`
}

type VideoInfo struct {
	VideoWidth  uint32
	VideoHeight uint32
	Rate        uint64
	Duration    float64
	Size        uint64
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

	fileDir := fmt.Sprintf("%s/%s", *uploadDir, uuid)
	if err := os.MkdirAll(fileDir, 0777); err != nil {
		writeUploadResponse(w, err)
		return
	}

	var filename string
	partIndex := req.FormValue(paramPartIndex)
	if len(partIndex) == 0 {
		filename = fmt.Sprintf("%s/%s", fileDir, headers.Filename)

	} else {
		filename = fmt.Sprintf("%s/%s_%05s", fileDir, uuid, partIndex)
	}
	outfile, err := os.Create(filename)
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
	log.Printf("upload file %s  to dir %s done ", filename, fileDir)
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

func executeFfprobeCommand(filename string, userId string, guid string) ([]byte, error) {
	if len(*ffprobePath) == 0 {
		log.Printf("ffprobe path is empty")
		return nil, fmt.Errorf("parameter error")
	}

	filePathName := userId + "/" + guid + "/" + filename

	args := fmt.Sprintf(" -v error -show_format -show_streams -print_format flat %s", filePathName)

	//ffprobe -v error -show_format -show_streams -print_format flat  test.mp4

	cmd := exec.Command(*ffprobePath, args)
	cmd.Run()
	data, err := cmd.Output()
	if err != nil {
		log.Printf("execute command ffprobe %s  failed", args)
		return nil, fmt.Errorf("execute command ffprobe %s  failed", args)
	}

	return data, nil
}

func parseFfprobeResult(result []byte) (*VideoInfo, error) {
	return nil, errors.New("test")
}

func ChunksDoneHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("ChunksDoneHandler")

	if req.Method != http.MethodPost {
		errorMsg := fmt.Sprintf("Method [%s] is not supported", req.Method)
		http.Error(w, errorMsg, http.StatusMethodNotAllowed)
	}
	uuid := req.FormValue(paramUuid)
	filename := req.FormValue(paramFileName)
	userid := req.FormValue(paramUserId)
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
		data, err := executeFfprobeCommand(filename, userid, uuid)
		if err != nil {
			log.Printf("execute ffprobe command failed")
			return
		} else {
			strData := string(data[:])
			strLines := strings.Split(strData, "\n")
			var videoInfo VideoInfo
			//get info from these lines
			/*
				streams.stream.0.index=0
				streams.stream.0.codec_name="h264"
				streams.stream.0.codec_long_name="H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10"
				streams.stream.0.profile="Main"
				streams.stream.0.codec_type="video"
				streams.stream.0.codec_time_base="1/30"
				streams.stream.0.codec_tag_string="avc1"
				streams.stream.0.codec_tag="0x31637661"
				streams.stream.0.width=800
				streams.stream.0.height=600
				streams.stream.0.coded_width=800
				streams.stream.0.coded_height=600
				streams.stream.0.has_b_frames=2
				streams.stream.0.sample_aspect_ratio="0:1"
				streams.stream.0.display_aspect_ratio="0:1"
				streams.stream.0.pix_fmt="yuv420p"
				streams.stream.0.level=31
				streams.stream.0.color_range="unknown"
				streams.stream.0.color_space="unknown"
				streams.stream.0.color_transfer="unknown"
				streams.stream.0.color_primaries="unknown"
				streams.stream.0.chroma_location="left"
				streams.stream.0.field_order="unknown"
				streams.stream.0.timecode="N/A"
				streams.stream.0.refs=1
				streams.stream.0.is_avc="true"
				streams.stream.0.nal_length_size="4"
				streams.stream.0.id="N/A"
				streams.stream.0.r_frame_rate="15/1"
				streams.stream.0.avg_frame_rate="15/1"
				streams.stream.0.time_base="1/15"
				streams.stream.0.start_pts=0
				streams.stream.0.start_time="0.000000"
				streams.stream.0.duration_ts=8002
				streams.stream.0.duration="533.466667"
				streams.stream.0.bit_rate="499122"
				streams.stream.0.max_bit_rate="N/A"
				streams.stream.0.bits_per_raw_sample="8"
				streams.stream.0.nb_frames="8002"
				streams.stream.0.nb_read_frames="N/A"
				streams.stream.0.nb_read_packets="N/A"
				streams.stream.0.disposition.default=1
				streams.stream.0.disposition.dub=0
				streams.stream.0.disposition.original=0
				streams.stream.0.disposition.comment=0
				streams.stream.0.disposition.lyrics=0
				streams.stream.0.disposition.karaoke=0
				streams.stream.0.disposition.forced=0
				streams.stream.0.disposition.hearing_impaired=0
				streams.stream.0.disposition.visual_impaired=0
				streams.stream.0.disposition.clean_effects=0
				streams.stream.0.disposition.attached_pic=0
				streams.stream.0.disposition.timed_thumbnails=0
				streams.stream.0.tags.creation_time="1970-01-01T00:00:00.000000Z"
				streams.stream.0.tags.language="und"
				streams.stream.0.tags.handler_name="VideoHandler"
				streams.stream.1.index=1
				streams.stream.1.codec_name="aac"
				streams.stream.1.codec_long_name="AAC (Advanced Audio Coding)"
				streams.stream.1.profile="LC"
				streams.stream.1.codec_type="audio"
				streams.stream.1.codec_time_base="1/44100"
				streams.stream.1.codec_tag_string="mp4a"
				streams.stream.1.codec_tag="0x6134706d"
				streams.stream.1.sample_fmt="fltp"
				streams.stream.1.sample_rate="44100"
				streams.stream.1.channels=2
				streams.stream.1.channel_layout="stereo"
				streams.stream.1.bits_per_sample=0
				streams.stream.1.id="N/A"
				streams.stream.1.r_frame_rate="0/0"
				streams.stream.1.avg_frame_rate="0/0"
				streams.stream.1.time_base="1/44100"
				streams.stream.1.start_pts=0
				streams.stream.1.start_time="0.000000"
				streams.stream.1.duration_ts=23535641
				streams.stream.1.duration="533.688005"
				streams.stream.1.bit_rate="96000"
				streams.stream.1.max_bit_rate="96000"
				streams.stream.1.bits_per_raw_sample="N/A"
				streams.stream.1.nb_frames="22984"
				streams.stream.1.nb_read_frames="N/A"
				streams.stream.1.nb_read_packets="N/A"
				streams.stream.1.disposition.default=1
				streams.stream.1.disposition.dub=0
				streams.stream.1.disposition.original=0
				streams.stream.1.disposition.comment=0
				streams.stream.1.disposition.lyrics=0
				streams.stream.1.disposition.karaoke=0
				streams.stream.1.disposition.forced=0
				streams.stream.1.disposition.hearing_impaired=0
				streams.stream.1.disposition.visual_impaired=0
				streams.stream.1.disposition.clean_effects=0
				streams.stream.1.disposition.attached_pic=0
				streams.stream.1.disposition.timed_thumbnails=0
				streams.stream.1.tags.creation_time="1970-01-01T00:00:00.000000Z"
				streams.stream.1.tags.language="und"
				streams.stream.1.tags.handler_name="SoundHandler"
				format.filename="test.mp4"
				format.nb_streams=2
				format.nb_programs=0
				format.format_name="mov,mp4,m4a,3gp,3g2,mj2"
				format.format_long_name="QuickTime / MOV"
				format.start_time="0.000000"
				format.duration="533.688000"
				format.size="39930706"
				format.bit_rate="598562"
				format.probe_score=100
				format.tags.major_brand="isom"
				format.tags.minor_version="512"
				format.tags.compatible_brands="isomiso2avc1mp41"
				format.tags.creation_time="1970-01-01T00:00:00.000000Z"
				format.tags.encoder="Lavf53.24.2
			*/
			for _, ele := range strLines {
				if strings.Contains(ele, ".width=") {
					ele_value := strings.Split(ele, "=")
					if len(ele_value) == 2 {
						videoWidth, err := strconv.Atoi(ele_value[1])
						if err != nil {
							log.Printf("convert video width failed %v", ele_value[1])
						} else {
							videoInfo.VideoWidth = uint32(videoWidth)
						}
					} else {
						log.Printf("read video width failed %v", ele)
					}
				}

				if strings.Contains(ele, ".height=") {
					ele_value := strings.Split(ele, "=")
					if len(ele_value) == 2 {
						videoHeight, err := strconv.Atoi(ele_value[1])
						if err != nil {
							log.Printf("convert video height failed %v", ele_value[1])
						} else {
							videoInfo.VideoHeight = uint32(videoHeight)
						}
					} else {
						log.Printf("read video height failed %v", ele)
					}
				}

				if strings.Contains(ele, ".duration=") {
					ele_value := strings.Split(ele, "=")
					if len(ele_value) == 2 {
						duration, err := strconv.ParseFloat(ele_value[1], 64)
						if err != nil {
							log.Printf("convert video duration failed %v", ele_value[1])
						} else {
							videoInfo.Duration = duration
						}
					} else {
						log.Printf("read video duration failed %v", ele)
					}
				}

				if strings.Contains(ele, "format.bit_rate=") {
					ele_value := strings.Split(ele, "=")
					if len(ele_value) == 2 {
						rate, err := strconv.ParseUint(ele_value[1], 10, 64)
						if err != nil {
							log.Printf("convert video bit rate failed %v", ele_value[1])
						} else {
							videoInfo.Rate = rate
						}
					} else {
						log.Printf("read video bit rate failed %v", ele)
					}
				}

				if strings.Contains(ele, "format.size=") {
					ele_value := strings.Split(ele, "=")
					if len(ele_value) == 2 {
						size, err := strconv.ParseUint(ele_value[1], 10, 64)
						if err != nil {
							log.Printf("convert video bit rate failed %v", ele_value[1])
						} else {
							videoInfo.Size = size
						}
					} else {
						log.Printf("read video bit rate failed %v", ele)
					}
				}

			}

			//todo
			//1. call api to tell the videoinfo to db server
			//2. call api to tell split server to split it into splits

		}
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
