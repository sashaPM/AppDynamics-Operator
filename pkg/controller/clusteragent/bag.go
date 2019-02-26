package clusteragent

type InstrumentationMethod string

const (
	Copy  InstrumentationMethod = "copy"
	Mount InstrumentationMethod = "mount"
)

type AppDBag struct {
	AppName                     string
	TierName                    string
	NodeName                    string
	AppID                       int
	TierID                      int
	NodeID                      int
	Account                     string
	GlobalAccount               string
	AccessKey                   string
	ControllerUrl               string
	ControllerPort              uint16
	SSLEnabled                  bool
	SystemSSLCert               string
	AgentSSLCert                string
	EventKey                    string
	EventServiceUrl             string
	RestAPICred                 string
	EventAPILimit               int
	PodSchemaName               string
	NodeSchemaName              string
	EventSchemaName             string
	ContainerSchemaName         string
	JobSchemaName               string
	LogSchemaName               string
	DashboardTemplatePath       string
	DashboardSuffix             string
	JavaAgentVersion            string
	AgentLabel                  string
	AppDAppLabel                string
	AppDTierLabel               string
	AppDAnalyticsLabel          string
	AgentMountName              string
	AgentMountPath              string
	AppLogMountName             string
	AppLogMountPath             string
	JDKMountName                string
	JDKMountPath                string
	NodeNamePrefix              string
	AnalyticsAgentUrl           string
	AnalyticsAgentImage         string
	AnalyticsAgentContainerName string
	AppDInitContainerName       string
	AppDJavaAttachImage         string
	AppDDotNetAttachImage       string
	AppDNodeJSAttachImage       string
	ProxyInfo                   string
	ProxyUser                   string
	ProxyPass                   string
	InstrumentationMethod       InstrumentationMethod
	InitContainerDir            string
	MetricsSyncInterval         int // Frequency of metrics pushes to the controller, sec
	SnapshotSyncInterval        int // Frequency of snapshot pushes to events api, sec
	AgentServerPort             int
	IncludeNsToInstrument       []string
	ExcludeNsToInstrument       []string
	DeploysToDashboard          []string
	IncludeNodesToInstrument    []string
	ExcludeNodesToInstrument    []string
}

func getDefaultProperties() *AppDBag {
	bag := AppDBag{AppName: "K8s-Cluster-Agent",
		TierName:                    "ClusterAgent",
		NodeName:                    "Node1",
		SystemSSLCert:               "/opt/appd/ssl/system.crt",
		AgentSSLCert:                "/opt/appd/ssl/agent.crt",
		EventAPILimit:               100,
		PodSchemaName:               "schema-pods",
		NodeSchemaName:              "schema-nodes",
		EventSchemaName:             "schema-events",
		ContainerSchemaName:         "schema-containers",
		JobSchemaName:               "schema-jobs",
		LogSchemaName:               "schema-logs",
		DashboardTemplatePath:       "/usr/local/go/src/github.com/sjeltuhin/clusterAgent/templates/k8s_dashboard_template.json",
		DashboardSuffix:             "SUMMARY",
		JavaAgentVersion:            "latest",
		AgentLabel:                  "appd-agent",
		AppDAppLabel:                "appd-app",
		AppDTierLabel:               "appd-tier",
		AppDAnalyticsLabel:          "appd-biq",
		AgentMountName:              "appd-agent-repo",
		AgentMountPath:              "/opt/appd",
		AppLogMountName:             "appd-volume",
		AppLogMountPath:             "/opt/appdlogs",
		JDKMountName:                "jdk-repo",
		JDKMountPath:                "$JAVA_HOME/lib",
		NodeNamePrefix:              "",
		AnalyticsAgentUrl:           "http://analytics-proxy:9090",
		AnalyticsAgentContainerName: "appd-analytics-agent",
		AppDInitContainerName:       "appd-agent-attach",
		AnalyticsAgentImage:         "sashaz/analytics-agent@sha256:ff776bdf3beed9f4bdf638d16b5a688d9e1c0fc124ce1282bef1851c122397e4",
		AppDJavaAttachImage:         "sashaz/java-agent-attach@sha256:b93f2018b091f4abfd2533e6c194c9e6ecf00fcae861c732f1b771dad1b26a80",
		AppDDotNetAttachImage:       "sashaz/dotnet-agent-attach@sha256:3f5d921eadfa227ffe072caa41e01c3c1fc882c5617ad45d808ffedaa20593a6",
		AppDNodeJSAttachImage:       "latest",
		ProxyInfo:                   "",
		ProxyUser:                   "",
		ProxyPass:                   "",
		InstrumentationMethod:       "mount",
		InitContainerDir:            "/opt/temp.",
		MetricsSyncInterval:         60,
		SnapshotSyncInterval:        15,
		AgentServerPort:             8989}
	return &bag
}
