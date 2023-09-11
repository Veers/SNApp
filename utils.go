package main

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
	VolumePath    string
	VolumeFolders []string
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

type DirInfo struct {
	Folder string
	Data   PairList
}

func NewDirInfo(initFolder string) DirInfo {
	return DirInfo{
		Folder: initFolder,
	}
}

var dirMap map[string]int64

type Pair struct {
	Key   string
	Value int64
}

func (p Pair) GetValue() string {
	return ByteCountSI(p.Value)
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
