package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	Percent                   int
	DateFormat_date           string
	DateFormat_time           string
	ThomeOutputFormat         string
	ThomeOutputFormat_success string
	ThomeOutputFormat_error   string
}

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
	thomeVars := make(map[string]interface{})

	thomeVars["ThomePercent"] = "percent"
	thomeVars["ThomeStatus"] = "status"

	// parse the template
	tmpl, _ := template.ParseFiles("templates/thomeinfo.tmpl")
	var tpl bytes.Buffer

	usage := NewDiskUsage(path)
	thomePercent := usage.Usage() * 100
	// fmt.Sprintf("%F %%", usage.Usage()*100)
	// thomePercent := 23.05643
	if int(thomePercent) >= params.Percent {
		thomeVars["ThomeStatus"] = params.ThomeOutputFormat_error
	} else {
		thomeVars["ThomeStatus"] = params.ThomeOutputFormat_success
	}

	if runtime.GOOS == "windows" {
		thomeVars["ThomeName"] = path
	} else {
		thomeVars["ThomeName"] = filepath.FromSlash(strings.Split(path, "/")[2])
	}

	thomeVars["ThomePercent"] = fmt.Sprintf("%.2f %%", thomePercent)
	tmpl.Execute(&tpl, thomeVars)
	return tpl.String()
}

func getDirectoryInfo(path string) string {
	dirThomeVars := make(map[string]interface{})

	fmt.Println(path)
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
	}
	files, err := f.Readdir(0)
	if err != nil {
		fmt.Println(err)
	}

	var size int64
	dirInfo := make([]string, 1)
	for _, v := range files {
		if v.IsDir() {
			var resDirStr string
			res, err := DirSize(filepath.FromSlash(path + "\\" + v.Name()))
			if err != nil {
				fmt.Println(err)
			}
			// fmt.Println(res)
			size = res
			fmt.Println(v.Name(), v.IsDir(), size)
			// fmt.Println(v.Name(), v.IsDir(), size)
			resDirStr = v.Name()
			dirInfo = append(dirInfo, resDirStr+" : "+ByteCountSI(size))
		}
	}

	dirInfoResText := strings.Join(dirInfo, "\r\n")
	dirThomeVars["ThomeDirInfo"] = dirInfoResText
	dirThomeVars["ThomeDirTitle"] = path

	// parse the template
	tmpl, _ := template.ParseFiles("templates/concretethomeinfo.tmpl")
	var tpl bytes.Buffer
	tmpl.Execute(&tpl, dirThomeVars)

	return tpl.String()
}

// func sendEmails() {
// 	smptConfigFile, _ := os.Open("smtp_conf.json")
// 	defer smptConfigFile.Close()
// 	smtpDecoder := json.NewDecoder(smptConfigFile)
// 	smtpConfiguration := SMTPConfiguration()
// 	err := smtpDecoder.Decode(&smtpConfiguration)
// 	if err != nil {
// 		fmt.Println("error:", err)
// 	}

// 	msg := "Hello geeks!!!"
// 	body := []byte(msg)
// 	auth := smtp.PlainAuth("", smtpConfiguration.from, smtpConfiguration.password, smtpConfiguration.host)
// 	err_smtp := smtp.SendMail(
// 		smtpConfiguration.server+":"+smtpConfiguration.port,
// 		auth,
// 		smtpConfiguration.from,
// 		smtpConfiguration.to,
// 		body)
// 	if err_smtp != nil {
// 		fmt.Println(err_smtp)
// 		os.Exit(1)
// 	}

// 	fmt.Println("Successfully sent mail to all user in toList")
// }

func main() {

	startDate := time.Now()

	// read config start
	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err := decoder.Decode(&configuration)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println(configuration.Volumes) // output: [UserA, UserB]
	// read config start

	// check disk (dir) size starts
	usage := NewDiskUsage("/")
	fmt.Println("Free:", usage.Free()/(KB*KB*KB))
	fmt.Println("Available:", usage.Available()/(KB*KB*KB))
	fmt.Println("Size:", usage.Size()/(KB*KB*KB))
	fmt.Println("Used:", usage.Used()/(KB*KB*KB))
	fmt.Println("Usage:", usage.Usage()*100, "%")
	// check disk (dir) size starts

	time.Sleep(2 * time.Second)

	// set := make(map[string]bool)

	thomeInfo := make([]string, len(configuration.Volumes))
	dirInfo := make([]string, 1)
	var thome string
	var thomePath string
	for _, s := range configuration.Volumes {
		if runtime.GOOS == "windows" {
			thome = filepath.FromSlash(s.VolumeGOOSLetter + ":")
			thomePath = filepath.FromSlash(s.VolumeGOOSLetter + ":/")
		} else {
			panic("ONLY WINDOWS IMPLEMENTATION")
			// thome = filepath.FromSlash(s.VolumeUNIXPath)
		}
		thomeInfo = append(thomeInfo, strings.TrimSpace(getThomeValues(thome, configuration.Params)))

		for _, f := range s.VolumeFolders {
			dirInfo = append(dirInfo, getDirectoryInfo(filepath.FromSlash(thomePath+f)))
		}
	}
	// sendEmails()

	thomeResText := strings.Join(thomeInfo, "\r\n")
	dirInfoResText := strings.Join(dirInfo, "\r\n")

	time.Sleep(2 * time.Second)

	endDate := time.Now()

	vars := make(map[string]interface{})
	vars["Time"] = startDate.Format(configuration.Params.DateFormat_time)
	vars["Date"] = startDate.Format(configuration.Params.DateFormat_date)
	vars["DateEnd"] = endDate.Format(configuration.Params.DateFormat_date)
	vars["TimeEnd"] = endDate.Format(configuration.Params.DateFormat_time)
	vars["ThomeInfo"] = strings.TrimSpace(thomeResText)
	vars["ProjectInfos"] = dirInfoResText
	// parse the template
	tmpl, _ := template.ParseFiles("templates/template.tmpl")
	// create a new file
	file1, _ := os.Create("greeting.txt")
	defer file1.Close()
	// apply the template to the vars map and write the result to file.
	tmpl.Execute(file1, vars)
}
