Installation
------------
1. Install [godep](https://github.com/tools/godep)
1. Setup your gopath
1. `git clone https://github.com/cloudfoundry-incubator/greenhouse-install-script-generator $GOPATH/src/github.com/cloudfoundry-incubator/greenhouse-install-script-generator`
1. `cd $GOPATH/src/github.com/cloudfoundry-incubator/greenhouse-install-script-generator &&
   godep restore`


Tests
-------------------
- `ginkgo -r`

Usage
-----
Sample for BOSH Lite:

`go run ./generate/generate.go -boshUrl https://admin:admin@192.168.50.4:25555 -outputDir /tmp/bosh-lite-install-bat -windowsPassword password -windowsUsername username`
