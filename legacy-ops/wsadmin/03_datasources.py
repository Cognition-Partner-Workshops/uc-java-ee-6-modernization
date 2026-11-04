# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
execfile('00_env.py')
provider = AdminConfig.getid('/Node:%s/JDBCProvider:DB2 Universal JDBC Driver Provider (XA)/' % nodeName)

# jdbc/TradeDataSource: XA, container-managed auth, used by JPA (persistence unit "daytrader") and TradeDirect
AdminTask.createDatasource(provider, '[-name TradeDataSource -jndiName jdbc/TradeDataSource '
    '-dataStoreHelperClassName com.ibm.websphere.rsadapter.DB2UniversalDataStoreHelper '
    '-containerManagedPersistence true -componentManagedAuthenticationAlias %s/TradeDataSourceAuthData '
    '-xaRecoveryAuthAlias %s/TradeDataSourceAuthData '
    '-configureResourceProperties [[databaseName java.lang.String TRADEDB] [driverType java.lang.Integer 4] '
    '[serverName java.lang.String tradeaixdb01.internal.example] [portNumber java.lang.Integer 50000]]]' % (nodeName, nodeName))

# jdbc/NoTxTradeDataSource: non-transactional (JPA non-jta-data-source for key generation)
AdminTask.createDatasource(provider, '[-name NoTxTradeDataSource -jndiName jdbc/NoTxTradeDataSource '
    '-dataStoreHelperClassName com.ibm.websphere.rsadapter.DB2UniversalDataStoreHelper '
    '-containerManagedPersistence false -componentManagedAuthenticationAlias %s/TradeDataSourceAuthData '
    '-configureResourceProperties [[databaseName java.lang.String TRADEDB] [driverType java.lang.Integer 4] '
    '[serverName java.lang.String tradeaixdb01.internal.example] [portNumber java.lang.Integer 50000]]]' % nodeName)

for dsName in ('TradeDataSource', 'NoTxTradeDataSource'):
    ds = AdminConfig.getid('/DataSource:%s/' % dsName)
    pool = AdminConfig.showAttribute(ds, 'connectionPool')
    AdminConfig.modify(pool, [['maxConnections', '50'], ['minConnections', '10'], ['agedTimeout', '0'], ['purgePolicy', 'EntirePool']])
    AdminConfig.modify(ds, [['statementCacheSize', '60']])
AdminConfig.save()
