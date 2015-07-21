Installation
------------
1. Install (godep)[https://github.com/tools/godep]
1. Setup your gopath
1. `git clone git@github.com:pivotal-cf/greenhouse-install-script-generator.git
   $GOPATH/src/github.com/pivotal-cf/greenhouse-install-script-generator`
1. `cd $GOPATH/src/github.com/pivotal-cf/greenhouse-install-script-generator &&
   godep restore`
1. `go run ./generate/generate.go BOSH_URL OUTPUT_DIR`


Tests
-------------------
- `ginkgo -r`
