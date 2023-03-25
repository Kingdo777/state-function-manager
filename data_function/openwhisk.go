package data_function

import "os"

const HomeDir = "/home/kingdo"
const OpenwhiskProjectHome = HomeDir + "/IdeaProjects/openwhisk"
const WskCli = OpenwhiskProjectHome + "/bin/wsk"
const WskConfigFile = HomeDir + "/.wskprops"

//const ApiHost = "222.20.94.67"
//const AuthName = "23bc46b1-71f6-4ed5-8c54-816aa4f8c502"
//const AuthPassword = "123zO3xZCLrMN6v2BKK1dXYFpXlPkccOFqm12CdAsMgRU4VrNZ9lyGVCGuMDGIwP"

var ApiHost = os.Getenv("Openwhisk_ApiHost")
var AuthName = os.Getenv("Openwhisk_AuthName")
var AuthPassword = os.Getenv("Openwhisk_AuthPassword")
