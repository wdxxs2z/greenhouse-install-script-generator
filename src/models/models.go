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

type ConsulProperties struct {
	RequireSSL  bool     `yaml:"require_ssl"`
	CACert      string   `yaml:"ca_cert"`
	AgentCert   string   `yaml:"agent_cert"`
	AgentKey    string   `yaml:"agent_key"`
	EncryptKeys []string `yaml:"encrypt_keys"`
	Agent       struct {
		Servers struct {
			Lan []string `yaml:"lan"`
		} `yaml:"servers"`
	} `yaml:"agent"`
}

type DiegoProperties struct {
	Rep struct {
		Zone string `yaml:"zone"`
	} `yaml:"rep"`
}

type EtcdProperties struct {
	Machines []string `yaml:"machines"`
}

type LoggregatorEndpoint struct {
	SharedSecret string `yaml:"shared_secret"`
}

type Properties struct {
	Consul      *ConsulProperties    `yaml:"consul"`
	Diego       *DiegoProperties     `yaml:"diego"`
	Etcd        *EtcdProperties      `yaml:"etcd"`
	Loggregator *LoggregatorEndpoint `yaml:"loggregator_endpoint"`
}

type Job struct {
	Properties *Properties `yaml:"properties"`
}

type Manifest struct {
	Jobs       []Job       `yaml:"jobs"`
	Properties *Properties `yaml:"properties"`
}
