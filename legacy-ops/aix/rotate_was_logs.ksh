#!/usr/bin/ksh
# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# Cron (wasadmin):
#   15 0 * * * /home/wasadmin/bin/rotate_was_logs.ksh >> /var/log/was_rotate.log 2>&1
#   0 */4 * * * /usr/bin/find /usr/IBM/WebSphere/AppServer/profiles/AppSrv01 -name "javacore.*" -mtime +7 -exec rm {} \;
. /home/wasadmin/bin/setenv_was.ksh
stamp=$(date +%Y%m%d)
archive=/waslogs/archive/$SERVER_NAME
mkdir -p $archive
for f in SystemOut.log SystemErr.log native_stdout.log native_stderr.log; do
  [ -s $LOG_ROOT/$f ] || continue
  cp -p $LOG_ROOT/$f $archive/$f.$stamp && cat /dev/null > $LOG_ROOT/$f
  gzip -9 $archive/$f.$stamp
done
find $archive -name "*.gz" -mtime +30 -exec rm {} \;
find $PROFILE_HOME/logs/ffdc -type f -mtime +14 -exec rm {} \;
