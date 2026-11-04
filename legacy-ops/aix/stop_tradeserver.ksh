#!/usr/bin/ksh
# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
. /home/wasadmin/bin/setenv_was.ksh
$PROFILE_HOME/bin/stopServer.sh $SERVER_NAME -profileName AppSrv01 -username wasadmin -password "$(cat /home/wasadmin/.was_admin_pw)" -timeout 300
rc=$?
if [ $rc -ne 0 ]; then
  pid=$(cat $LOG_ROOT/$SERVER_NAME.pid 2>/dev/null)
  [ -n "$pid" ] && kill -9 $pid && echo "force-killed $SERVER_NAME ($pid)"
fi
logger -t daytrader "stopServer $SERVER_NAME rc=$rc"
exit $rc
