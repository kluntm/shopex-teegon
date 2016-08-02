INSTALLPATH=/opt/shopex/teegon
PACKAGENAME=mqservice.tar.gz
MAINNAME=mqservice
CONFIG_DIR=etc
LOG_DIR=log

all: buildversion.go
	go build -o ${MAINNAME}
	test -d ./bin || mkdir -p ./bin
	\cp -f ${MAINNAME} ./bin
	rm -f ${MAINNAME}

buildversion.go:
	echo package main > buildversion.go
	echo const BuildVersion = \"`git --no-pager log --pretty="%h" -n 1`\" >> buildversion.go

clean:
	rm -f ./bin/${MAINNAME}
	rm -f buildversion.go

install:
	test -d ${INSTALLPATH}/bin || mkdir -p ${INSTALLPATH}/bin
	test -d ${INSTALLPATH}/${LOG_DIR} || mkdir -p ${INSTALLPATH}/${LOG_DIR}
	\cp -f ./bin/${MAINNAME} ${INSTALLPATH}/bin
	\cp -rf ./${CONFIG_DIR} ${INSTALLPATH}

package:
	test -d ./log || mkdir -p ./log
	test ! -f ${PACKAGENAME} || rm ${PACKAGENAME}
	tar zcf ${PACKAGENAME} ./bin/${MAINNAME} ./${CONFIG_DIR} ./${LOG_DIR}
