# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# IBM MQ messaging provider resources (the Liberty config uses the embedded wasJmsServer instead).
execfile('00_env.py')
scope = 'Node=%s,Server=%s' % (nodeName, serverName)
conn = '-qmgrName %s -wmqTransportType CLIENT -qmgrHostname %s -qmgrPortNumber %d -qmgrSvrconnChannel %s' % (mqQueueManager, mqHost, mqPort, mqChannel)

AdminTask.createWMQConnectionFactory(scope, '[-type QCF -name TradeBrokerQCF -jndiName jms/TradeBrokerQCF %s '
    '-containerAuthAlias TradeAdminAuthData -xaRecoveryAuthAlias TradeAdminAuthData]' % conn)
AdminTask.createWMQConnectionFactory(scope, '[-type TCF -name TradeStreamerTCF -jndiName jms/TradeStreamerTCF %s '
    '-containerAuthAlias TradeAdminAuthData]' % conn)

AdminTask.createWMQQueue(scope, '[-name TradeBrokerQueue -jndiName jms/TradeBrokerQueue -queueName TRADE.BROKER.QUEUE -qmgr %s]' % mqQueueManager)
AdminTask.createWMQTopic(scope, '[-name TradeStreamerTopic -jndiName jms/TradeStreamerTopic -topicName TradeStreamerTopic]')

# Activation specs bound by ibm-ejb-jar-bnd.xml to the two MDBs
AdminTask.createWMQActivationSpec(scope, '[-name TradeBrokerMDB -jndiName eis/TradeBrokerMDB -destinationJndiName jms/TradeBrokerQueue '
    '-destinationType javax.jms.Queue -authAlias TradeAdminAuthData %s -maxPoolSize 10]' % conn)    # -> DTBroker3MDB
AdminTask.createWMQActivationSpec(scope, '[-name TradeStreamerMDB -jndiName eis/TradeStreamerMDB -destinationJndiName jms/TradeStreamerTopic '
    '-destinationType javax.jms.Topic -authAlias TradeAdminAuthData %s -maxPoolSize 1]' % conn)     # -> DTStreamer3MDB
AdminConfig.save()
