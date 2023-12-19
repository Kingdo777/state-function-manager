package state_function

import "os"

const HomeDir = "/home/kingdo"
const OpenwhiskProjectHome = "../../../OpenWhisk/project-src"
const WskCli = OpenwhiskProjectHome + "/bin/wsk"
const WskConfigFile = HomeDir + "/.wskprops"

func ApiHost() string {
	var value, ok = os.LookupEnv("Openwhisk_ApiHost")
	if !ok {
		value = "222.20.94.67"
	}
	return value
}

func AuthName() string {
	var value, ok = os.LookupEnv("Openwhisk_AuthName")
	if !ok {
		value = "23bc46b1-71f6-4ed5-8c54-816aa4f8c502"
	}
	return value
}

func AuthPassword() string {
	var value, ok = os.LookupEnv("Openwhisk_AuthPassword")
	if !ok {
		value = "123zO3xZCLrMN6v2BKK1dXYFpXlPkccOFqm12CdAsMgRU4VrNZ9lyGVCGuMDGIwP"
	}
	return value
}
