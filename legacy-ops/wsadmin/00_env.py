# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# Usage on the deployment manager (illustrative):
#   /usr/IBM/WebSphere/AppServer/profiles/Dmgr01/bin/wsadmin.sh -lang jython -f 00_env.py
cellName = 'TRADECELL01'
nodeName = 'tradeaix01Node01'
serverName = 'TradeServer1'
clusterName = None  # single server; a real estate would usually target a cluster
scopeId = AdminConfig.getid('/Cell:%s/Node:%s/Server:%s/' % (cellName, nodeName, serverName))
nodeScope = AdminConfig.getid('/Cell:%s/Node:%s/' % (cellName, nodeName))
db2DriverPath = '/opt/IBM/db2/V11.5/java'
mqQueueManager = 'QMTRADE1'
mqHost = 'tradeaixmq01.internal.example'
mqPort = 1414
mqChannel = 'TRADE.SVRCONN'
