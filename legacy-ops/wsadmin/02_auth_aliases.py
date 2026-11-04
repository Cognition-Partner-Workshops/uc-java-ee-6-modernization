# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# J2C authentication aliases. Passwords come from the change ticket at run time, never from this file.
import sys
execfile('00_env.py')
dbUser, dbPassword, mqUser, mqPassword = sys.argv[0], sys.argv[1], sys.argv[2], sys.argv[3]

AdminTask.createAuthDataEntry('[-alias TradeDataSourceAuthData -user %s -password %s -description "DayTrader Db2 runtime user"]' % (dbUser, dbPassword))
AdminTask.createAuthDataEntry('[-alias TradeAdminAuthData -user %s -password %s -description "DayTrader MQ / MDB activation user"]' % (mqUser, mqPassword))
AdminConfig.save()
