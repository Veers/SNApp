package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
	"unsafe"
)

type DiskUsage struct {
	freeBytes  int64
	totalBytes int64
	availBytes int64
}

// NewDiskUsages returns an object holding the disk usage of volumePath
// or nil in case of error (invalid path, etc)
func NewDiskUsage(volumePath string) *DiskUsage {

	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	du := &DiskUsage{}

	c.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(volumePath))),
		uintptr(unsafe.Pointer(&du.freeBytes)),
		uintptr(unsafe.Pointer(&du.totalBytes)),
		uintptr(unsafe.Pointer(&du.availBytes)))

	return du
}

// Free returns total free bytes on file system
func (du *DiskUsage) Free() uint64 {
	return uint64(du.freeBytes)
}

// Available returns total available bytes on file system to an unprivileged user
func (du *DiskUsage) Available() uint64 {
	return uint64(du.availBytes)
}

// Size returns total size of the file system
func (du *DiskUsage) Size() uint64 {
	return uint64(du.totalBytes)
}

// Used returns total bytes used in file system
func (du *DiskUsage) Used() uint64 {
	return du.Size() - du.Free()
}

// Usage returns percentage of use on the file system
func (du *DiskUsage) Usage() float32 {
	return float32(du.Used()) / float32(du.Size())
}

var KB = uint64(1024)

type SMTPConfiguration struct {
	server   string
	port     int
	from     string
	login    string
	password string
	reply    string
	to       []string
}

type DataVolume struct {
	VolumeGOOSLetter string
	VolumeUNIXPath   string
	VolumeFolders    []string
}

type Configuration struct {
	Volumes  []DataVolume
	MailList []string
	Params   Params
}

type Params struct {
	Percent                     int
	DateFormat_date             string
	DateFormat_time             string
	VolumeOutputMessage_success string
	VolumeOutputMessage_error   string
	ThreadsPerVolumes           int
	SortFolders                 string
}

var dirMap map[string]int64

type Pair struct {
	Key   string
	Value int64
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func getThomeValues(path string, params Params) string {
	volumeTemplateVars := make(map[string]interface{})

	volumeTemplateVars["Message"] = "."

	// parse the template
	tmpl, tmplErr := template.ParseFiles("templates/volumeinfo.tmpl")
	if tmplErr != nil {
		log.Println(tmplErr)
	}
	var tpl bytes.Buffer

	usage := NewDiskUsage(path)
	thomePercent := usage.Usage() * 100
	if int(thomePercent) >= params.Percent {
		volumeTemplateVars["Message"] = params.VolumeOutputMessage_error
	} else {
		volumeTemplateVars["Message"] = params.VolumeOutputMessage_success
	}

	if runtime.GOOS == "windows" {
		volumeTemplateVars["VolumeName"] = path
	} else {
		volumeTemplateVars["VolumeName"] = filepath.FromSlash(strings.Split(path, "/")[2])
	}

	volumeTemplateVars["CapacityPercent"] = fmt.Sprintf("%.2f %%", thomePercent)
	tmpl.Execute(&tpl, volumeTemplateVars)
	return tpl.String()
}

func rankBySize(data map[string]int64, sortType string) PairList {
	pl := make(PairList, len(data))
	i := 0
	for k, v := range data {
		pl[i] = Pair{k, v}
		i++
	}
	// bad code
	if sortType == "ASC" {
		sort.Sort(pl)
	}
	if sortType == "DESC" {
		sort.Sort(sort.Reverse(pl))
	}
	return pl
}

func getDirectoryInfo(path string, sortType string) []string {

	f, err := os.Open(path)
	if err != nil {
		log.Println(err)
	}
	files, err := f.Readdir(0)
	if err != nil {
		log.Println(err)
	}

	var size int64
	dirMap = make(map[string]int64)
	dirInfo := make([]string, 1)
	for _, v := range files {
		if v.IsDir() {
			var resDirStr string
			res, err := DirSize(filepath.FromSlash(path + "\\" + v.Name()))
			if err != nil {
				log.Println(err)
			}
			size = res
			resDirStr = v.Name()
			dirMap[resDirStr] = size
			// dirInfo = append(dirInfo, resDirStr+" : "+ByteCountSI(size))
		}
	}
	res := rankBySize(dirMap, sortType)

	for _, d := range res {
		dirInfo = append(dirInfo, d.Key+" "+ByteCountSI(d.Value))
	}

	return dirInfo
}

func sendEmails(msg []byte) {
	smtpConfigFile, smtpErr := os.Open("smtp_conf.json")
	if smtpErr != nil {
		log.Println(smtpErr)
	}

	defer smtpConfigFile.Close()
	smtpDecoder := json.NewDecoder(smtpConfigFile)
	smtpConfiguration := SMTPConfiguration{}
	err := smtpDecoder.Decode(&smtpConfiguration)
	if err != nil {
		log.Println("error:", err)
	}

	body := []byte(msg)
	auth := smtp.PlainAuth("", smtpConfiguration.login, smtpConfiguration.password, smtpConfiguration.server)
	err_smtp := smtp.SendMail(
		smtpConfiguration.server+":"+strconv.Itoa(smtpConfiguration.port),
		auth,
		smtpConfiguration.from,
		smtpConfiguration.to,
		body)
	if err_smtp != nil {
		log.Println(err_smtp)
		os.Exit(1)
	}

	log.Println("Successfully sent mail to all user in toList")
}

func main() {
	// setup logs
	logFile, logErr := os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if logErr != nil {
		log.Fatal(logErr)
	}
	log.SetOutput(logFile)

	startDate := time.Now()

	// read config start
	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err := decoder.Decode(&configuration)
	if err != nil {
		log.Println("error:", err)
	}
	// read config start

	time.Sleep(2 * time.Second)

	volumesInfo := make([]string, 0)
	var thome string
	var thomePath string
	m := make(map[string][]string)
	// concurrency shit
	var wg sync.WaitGroup
	wg.Add(len(configuration.Volumes))
	// var volumeInfo string
	for _, s := range configuration.Volumes {
		if runtime.GOOS == "windows" {
			thome = filepath.FromSlash(s.VolumeGOOSLetter + ":")
			thomePath = filepath.FromSlash(s.VolumeGOOSLetter + ":/")
		} else {
			panic("ONLY WINDOWS IMPLEMENTATION")
		}
		go func(thome string, params Params) {
			volumesInfo = append(volumesInfo, getThomeValues(thome, params))
			wg.Done()
		}(thome, configuration.Params)

		var dirWg sync.WaitGroup
		dirWg.Add(len(s.VolumeFolders))
		for _, f := range s.VolumeFolders {
			go func(f string) {
				m[filepath.FromSlash(thomePath+f)] = getDirectoryInfo(filepath.FromSlash(thomePath+f), configuration.Params.SortFolders)
				dirWg.Done()
			}(f)
		}
		dirWg.Wait()
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	endDate := time.Now()

	vars := make(map[string]interface{})
	vars["Time"] = startDate.Format(configuration.Params.DateFormat_time)
	vars["Date"] = startDate.Format(configuration.Params.DateFormat_date)
	vars["DateEnd"] = endDate.Format(configuration.Params.DateFormat_date)
	vars["TimeEnd"] = endDate.Format(configuration.Params.DateFormat_time)
	vars["Volumes"] = volumesInfo
	vars["Folders"] = m
	vars["c"] = formatExecutionTime(endDate.Sub(startDate))
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	// parse the template
	tmpl, err := template.ParseFiles("templates/template.tmpl")
	if err != nil {
		log.Panic(err)
		panic(err)
	}
	// create a new file
	file1, _ := os.Create("greeting.txt")
	var mailTpl bytes.Buffer
	defer file1.Close()
	// apply the template to the vars map and write the result to file.
	tmpl.Execute(file1, vars)
	tmpl.Execute(&mailTpl, vars)

	// sendEmails(mailTpl.Bytes())
}

func formatExecutionTime(d time.Duration) string {
	d = d.Round(time.Millisecond)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d:%04d", h, m, s, ms)
}
