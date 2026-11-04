#!/usr/bin/ksh
# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
. /home/wasadmin/bin/setenv_was.ksh
if [ "$(whoami)" != "wasadmin" ]; then echo "run as wasadmin" >&2; exit 1; fi
$PROFILE_HOME/bin/startServer.sh $SERVER_NAME -profileName AppSrv01 -timeout 600
rc=$?
grep -q "open for e-business" $LOG_ROOT/startServer.log || rc=2
logger -t daytrader "startServer $SERVER_NAME rc=$rc"
exit $rc
