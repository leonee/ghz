package runner

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jhump/protoreflect/desc"
)

// call template data
type callTemplateData struct {
	WorkerID           string // unique worker ID
	RequestNumber      int64  // unique incremented request number for each request
	FullyQualifiedName string // fully-qualified name of the method call
	MethodName         string // shorter call method name
	ServiceName        string // the service name
	InputName          string // name of the input message type
	OutputName         string // name of the output message type
	IsClientStreaming  bool   // whether this call is client streaming
	IsServerStreaming  bool   // whether this call is server streaming
	Timestamp          string // timestamp of the call in RFC3339 format
	TimestampUnix      int64  // timestamp of the call as unix time in seconds
	TimestampUnixMilli int64  // timestamp of the call as unix time in milliseconds
	TimestampUnixNano  int64  // timestamp of the call as unix time in nanoseconds
	UUID               string // generated UUIDv4 for each call
}

// newCallTemplateData returns new call template data
func newCallTemplateData(mtd *desc.MethodDescriptor, workerID string, reqNum int64) *callTemplateData {
	now := time.Now()
	newUUID, _ := uuid.NewRandom()

	return &callTemplateData{
		WorkerID:           workerID,
		RequestNumber:      reqNum,
		FullyQualifiedName: mtd.GetFullyQualifiedName(),
		MethodName:         mtd.GetName(),
		ServiceName:        mtd.GetService().GetName(),
		InputName:          mtd.GetInputType().GetName(),
		OutputName:         mtd.GetOutputType().GetName(),
		IsClientStreaming:  mtd.IsClientStreaming(),
		IsServerStreaming:  mtd.IsServerStreaming(),
		Timestamp:          now.Format(time.RFC3339),
		TimestampUnix:      now.Unix(),
		TimestampUnixMilli: now.UnixNano() / 1000000,
		TimestampUnixNano:  now.UnixNano(),
		UUID:               newUUID.String(),
	}
}

func (td *callTemplateData) execute(data string) (*bytes.Buffer, error) {
	t := template.Must(template.New("call_template_data").Funcs(template.FuncMap{
		// Read file content into string
		"Read": func(file string) (string, error) {
			b, err := ioutil.ReadFile(file)
			if err != nil {
				return "", err
			}
			s := strings.TrimSpace(string(b))
			return s, nil
		},
		// Generate sequence number by step, include start, exclude end
		"Range": func(start, end, step int) []string {
			if step <= 0 || start >= step {
				return []string{}
			}
			res := make([]string, 0, (end-start)/step+1)
			for start < end {
				res = append(res, strconv.Itoa(start))
				start += step
			}
			return res
		},
		// Covert string to int
		"ToInt": func(str string) (int, error) {
			res, err := strconv.Atoi(str)
			if err != nil {
				return 0, err
			}
			return res, nil
		},
		// Alias of strings.Join
		"Join": strings.Join,
		// Alias of strings.Split
		"Split": strings.Split,
		// Base64 encode input data
		"B64Encode": func(data string) string {
			s := base64.StdEncoding.EncodeToString([]byte(data))
			return s
		},
		// Base64 decode input data
		"B64Decode": func(data string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		// List all files in the path
		"ListFile": func(dirPath string) ([]string, error) {
			files, err := ioutil.ReadDir(dirPath)
			if err != nil {
				return []string{}, err
			}
			var paths []string
			for _, f := range files {
				if !f.IsDir() {
					paths = append(paths, path.Join(dirPath, f.Name()))
				}
			}
			return paths, nil
		},
		// Randomly choose one value from values
		"RandomChoice": func(values []string) (string, error) {
			if len(values) < 1 {
				return "", errors.New("values is empty")
			}
			value := values[rand.Intn(len(values))]
			return value, nil
		},
		// Randomly returns a continuous non-empty sub sequence of input
		"RandomSlice": func(values []string) ([]string, error) {
			if len(values) < 1 {
				return []string{}, errors.New("values is empty")
			}
			start := rand.Intn(len(values))
			end := rand.Intn(len(values))
			if start > end {
				start, end = end, start
			} else if start == end {
				if start == 0 {
					end = end + 1
				} else {
					start = start - 1
				}
			}
			return values[start:end], nil
		},
		// Randomly returns a continuous sub sequence with length k of input
		"RandomSliceK": func(values []string, k int) ([]string, error) {
			if len(values) < k {
				return []string{}, errors.New("values must be longer than k")
			}
			if len(values) == k {
				return values, nil
			}
			start := rand.Intn(len(values) - k)
			end := start + k
			return values[start:end], nil
		},
		// Randomly returns a int between [n,m), where m > n >= 0, returns string
		"RandomInt": func(n, m int) (string, error) {
			if n < 0 || m < 1 {
				return "", errors.New("must be m > n >= 0")
			}
			if m <= n {
				return "", errors.New("m must > n")
			}
			value := strconv.Itoa(rand.Intn(m-n) + n)
			return value, nil
		},
		// Shuffle the input and returns
		"Shuffle": func(values []string) []string {
			rand.Shuffle(len(values), func(i, j int) { values[i], values[j] = values[j], values[i] })
			return values
		},
		// RoundRobin-ly select one value from values, mod with RequestNumber
		"RoundRobin": func(values []string) (string, error) {
			if len(values) < 1 {
				return "", errors.New("values is empty")
			}
			value := values[td.RequestNumber%int64(len(values))]
			return value, nil
		},
	}).Parse(data))
	var tpl bytes.Buffer
	err := t.Execute(&tpl, td)
	return &tpl, err
}

func (td *callTemplateData) executeData(data string) ([]byte, error) {
	if len(data) > 0 {
		input := []byte(data)
		tpl, err := td.execute(data)
		if err == nil {
			input = tpl.Bytes()
		}

		return input, nil
	}

	return []byte{}, nil
}

func (td *callTemplateData) executeMetadata(metadata string) (*map[string]string, error) {
	var mdMap map[string]string

	if len(metadata) > 0 {
		input := []byte(metadata)
		tpl, err := td.execute(metadata)
		if err == nil {
			input = tpl.Bytes()
		}

		err = json.Unmarshal(input, &mdMap)
		if err != nil {
			return nil, err
		}
	}

	return &mdMap, nil
}
