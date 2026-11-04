# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
execfile('00_env.py')
ear = '/opt/IBM/deploy/daytrader3/daytrader3-ee6.ear'
target = 'WebSphere:cell=%s,node=%s,server=%s' % (cellName, nodeName, serverName)

AdminApp.install(ear, ['-appname', 'daytrader3', '-nopreCompileJSPs', '-distributeApp', '-usedefaultbindings',
    '-MapModulesToServers', [['.*', '.*', target]],
    '-MapWebModToVH', [['.*', '.*', 'default_host']],
    '-CtxRootForWebMod', [['.*', 'web.war,.*', '/daytrader'], ['.*', 'Rest.war,.*', '/rest']],
    '-MapResRefToEJB', [
        ['.*', '.*', 'TradeSLSBBean', '.*', 'jms/QueueConnectionFactory', 'javax.jms.QueueConnectionFactory', 'jms/TradeBrokerQCF', 'TradeAdminAuthData', '', ''],
        ['.*', '.*', 'TradeSLSBBean', '.*', 'jms/TopicConnectionFactory', 'javax.jms.TopicConnectionFactory', 'jms/TradeStreamerTCF', 'TradeAdminAuthData', '', ''],
        ['.*', 'web.war,.*', '', '.*', 'jdbc/TradeDataSource', 'javax.sql.DataSource', 'jdbc/TradeDataSource', 'TradeDataSourceAuthData', '', '']],
    '-MapMessageDestinationRefToEJB', [
        ['.*', '.*', 'TradeSLSBBean', 'jms/TradeBrokerQueue', 'jms/TradeBrokerQueue'],
        ['.*', '.*', 'TradeSLSBBean', 'jms/TradeStreamerTopic', 'jms/TradeStreamerTopic']],
    '-BindJndiForEJBMessageBinding', [
        ['.*', '.*', 'DTBroker3MDB', '.*', '', 'jms/TradeBrokerQueue', 'eis/TradeBrokerMDB', 'TradeAdminAuthData'],
        ['.*', '.*', 'DTStreamer3MDB', '.*', '', 'jms/TradeStreamerTopic', 'eis/TradeStreamerMDB', 'TradeAdminAuthData']],
    '-MapSharedLibForMod', [['daytrader3', 'META-INF/application.xml', 'DayTraderSharedLib']]])

dep = AdminConfig.getid('/Deployment:daytrader3/')
AdminConfig.modify(AdminConfig.showAttribute(dep, 'deployedObject'), [['warClassLoaderPolicy', 'SINGLE'], ['startingWeight', '10']])
AdminConfig.save()
AdminControl.invoke(AdminControl.queryNames('type=ApplicationManager,process=%s,*' % serverName), 'startApplication', 'daytrader3')
