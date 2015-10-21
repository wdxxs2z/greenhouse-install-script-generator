package models

type Release struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type IndexDeployment struct {
	Name     string    `json:"name"`
	Releases []Release `json:"releases"`
}

type ShowDeployment struct {
	Manifest string `json:"manifest"`
}

type InstallerArguments struct {
	ConsulRequireSSL bool
	ConsulIPs        string
	EtcdCluster      string
	Zone             string
	SharedSecret     string
	Username         string
	Password         string
	SyslogHostIP     string
	SyslogPort       string
	BbsRequireSsl    bool
}
