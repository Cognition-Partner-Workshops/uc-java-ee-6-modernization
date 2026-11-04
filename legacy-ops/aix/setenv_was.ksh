#!/usr/bin/ksh
# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# Sourced by the start/stop wrappers. AIX 7.2, WAS ND 9.0.5, IBM Java 8.
export WAS_HOME=/usr/IBM/WebSphere/AppServer
export PROFILE_HOME=$WAS_HOME/profiles/AppSrv01
export SERVER_NAME=TradeServer1
export LOG_ROOT=$PROFILE_HOME/logs/$SERVER_NAME
export LDR_CNTRL=MAXDATA=0xB0000000@DSA
export AIXTHREAD_SCOPE=S
export EXTSHM=ON

ulimit -n 65536      # open files (Db2 + MQ client connections, JSP class files)
ulimit -u 4096       # processes / threads
ulimit -c unlimited  # keep core files for IBM support
ulimit -d unlimited
ulimit -s 2097152
