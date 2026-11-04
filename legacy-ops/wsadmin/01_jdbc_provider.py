# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
execfile('00_env.py')

AdminTask.createJDBCProvider('[-scope Node=%s -databaseType DB2 -providerType "DB2 Universal JDBC Driver Provider" '
    '-implementationType "XA data source" -name "DB2 Universal JDBC Driver Provider (XA)" '
    '-description "Db2 type 4 (db2jcc4.jar) for DayTrader" '
    '-classpath [${DB2UNIVERSAL_JDBC_DRIVER_PATH}/db2jcc4.jar ${DB2UNIVERSAL_JDBC_DRIVER_PATH}/db2jcc_license_cu.jar] ]' % nodeName)
AdminTask.setVariable('[-variableName DB2UNIVERSAL_JDBC_DRIVER_PATH -variableValue %s -scope Node=%s]' % (db2DriverPath, nodeName))
AdminConfig.save()
