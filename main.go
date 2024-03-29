package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"golang.org/x/sys/unix"
)

var stat unix.Statfs_t

// NewDiskUsages returns an object holding the disk usage of volumePath
// or nil in case of error (invalid path, etc)
func getDiskUsage(volumePath string) (float64, error) {
	wd, err := os.Getwd()
	if err != nil {
		return -1.0, err
	}
	unix.Statfs(wd, &stat)

	// Available blocks * size per block = available space in bytes
	var fullSize = stat.Blocks * uint64(stat.Bsize)
	var freeSize = stat.Bfree * uint64(stat.Bsize)
	var percent = (float64(fullSize-freeSize) / float64(fullSize)) * 100
	ratio := math.Pow(10, float64(2))
	var roundPercent = math.Round(percent*ratio) / ratio
	return roundPercent, nil
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

func getThomeValues(n int, c chan string, config Configuration) {
	volumeTemplateVars := make(map[string]interface{})

	volumeTemplateVars["Message"] = "."

	// parse the template
	tmpl, tmplErr := template.ParseFiles("templates/volumeinfo.tmpl")
	if tmplErr != nil {
		log.Println(tmplErr)
	}
	var tpl bytes.Buffer

	app := "df"
	arg0 := "-Ph"
	// arg0 := "-PhT" // use T only with fs type third flag
	// arg1 := "acfs"

	cmd := exec.Command(app, arg0)
	stdout, errr := cmd.Output()

	if errr != nil {
		fmt.Println(errr.Error())
	}

	var sout = string(stdout)
	soutarr := strings.Split(sout, "\n")

	// loop over thomes
	var usage float64
	var err error
	for i := 0; i < n; i++ {
		var path = filepath.FromSlash(config.Volumes[i].VolumePath)
		var dflag = false
		var index_pos = -1
		for i := range soutarr {
			if strings.Contains(soutarr[i], path) {
				dflag = true
				index_pos = i
			}
		}
		if dflag && index_pos != -1 {
			usage = ExtractPercentrageFromDFStr(soutarr[index_pos])
		} else {
			usage, err = getDiskUsage(path)
		}

		if err != nil {
			volumeTemplateVars["Message"] = "FATAL ERROR!"
		}
		if int(usage) >= config.Params.Percent {
			volumeTemplateVars["Message"] = config.Params.VolumeOutputMessage_error
		} else {
			volumeTemplateVars["Message"] = config.Params.VolumeOutputMessage_success
		}

		volumeTemplateVars["VolumeName"] = path
		volumeTemplateVars["CapacityPercent"] = fmt.Sprintf("%.2f%%", usage)
		tmpl.Execute(&tpl, volumeTemplateVars)
		c <- tpl.String()
		// clear buffer for next chan
		tpl.Truncate(0)
	}
	close(c)
}

func ExtractPercentrageFromDFStr(s string) float64 {

	// Define a regular expression pattern to match the percentage (digits followed by '%')
	percentPattern := `(\d+)\%`

	// Compile the regular expression
	regex := regexp.MustCompile(percentPattern)

	// Find the first match of the percentage in the input string
	match := regex.FindStringSubmatch(s)

	if len(match) >= 2 {
		// Extract the percentage value from the match
		percentage := match[1]
		percentageFloat, err := strconv.ParseFloat(percentage, 64)
		if err != nil {
			fmt.Println("Error converting string to float:", err)
		}
		return percentageFloat
	} else {
		fmt.Println("Percentage not found in the input string.")
		return -1.0
	}
}

func rankBySize(data map[string]string, sortType string) PairList {
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

func getDirectoryInfo(c chan []DirInfo, configuration Configuration, sortType string) {
	var thomePath string
	returnedDirInfo := make([]DirInfo, 0)
	// loop over thomes
	for _, s := range configuration.Volumes {
		thomePath = filepath.FromSlash(s.VolumePath + "/")
		// make return struct for each thome instance
		di := NewDirInfo(thomePath)
		di.Data = PairList{}
		for _, f := range s.VolumeFolders {
			// append to info each folder with it size
			var fullPath = fmt.Sprintf("%s%s", thomePath, f)
			dds, err := IDirSize(fullPath)
			if err != nil {
				log.Printf("Error while directory size counting: %s", err)
				continue
			}
			di.Data = append(di.Data, Pair{fullPath, dds})
		}
		returnedDirInfo = append(returnedDirInfo, di)
	}
	c <- returnedDirInfo
	close(c)
}

func IDirSize(path string) (string, error) {
	app := "du"
	arg0 := "-sh"

	cmd := exec.Command(app, arg0, path)
	stdout, errr := cmd.Output()

	if errr != nil {
		fmt.Println(errr.Error())
	}
	var sout = string(stdout)
	return strings.Split(sout, "\t")[0], nil
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
	// set start time
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
	// read config end

	time.Sleep(2 * time.Second)

	// get info about root paths (thomes)
	volumesInfo := make([]string, 0)
	dirsInfo := make([]DirInfo, 0)
	c := make(chan string, len(configuration.Volumes))
	go getThomeValues(cap(c), c, configuration)

	// print from channels to template
	for i := range c {
		volumesInfo = append(volumesInfo, i)
	}

	// get subdirs info
	var dch = GetCountDirChannels(configuration.Volumes)
	dc := make(chan []DirInfo, dch)
	go getDirectoryInfo(dc, configuration, configuration.Params.SortFolders)
	rres := <-dc

	if len(rres) > 0 {
		dirsInfo = rres
	}
	endDate := time.Now()

	// generate report
	vars := make(map[string]interface{})
	vars["Time"] = startDate.Format(configuration.Params.DateFormat_time)
	vars["Date"] = startDate.Format(configuration.Params.DateFormat_date)
	vars["DateEnd"] = endDate.Format(configuration.Params.DateFormat_date)
	vars["TimeEnd"] = endDate.Format(configuration.Params.DateFormat_time)
	vars["Volumes"] = volumesInfo
	vars["Folders"] = dirsInfo
	vars["c"] = formatExecutionTime(endDate.Sub(startDate))
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

	// send emails
	sendEmails(mailTpl.Bytes())
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

func GetCountDirChannels(volumes []DataVolume) int {
	var cnt = 0
	for _, item := range volumes {
		cnt += len(item.VolumeFolders)
	}
	return cnt
}
