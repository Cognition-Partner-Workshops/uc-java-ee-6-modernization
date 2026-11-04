# SYNTHETIC - illustrative traditional WebSphere (tWAS 8.5.5/9.0) on AIX artefact for the migration demo.
# Not the runtime of this repo (that is Liberty: daytrader3-ee6-wlpcfg/servers/daytrader3_Sample/server.xml).
# Not executed anywhere (no CI, no Makefile target). JNDI names match the application's real bindings.
# Shared library for jars kept outside the EAR (the Liberty equivalent is the Daytrader3_SampleDerbyLibs library).
execfile('00_env.py')
AdminConfig.create('Library', AdminConfig.getid('/Node:%s/' % nodeName),
    [['name', 'DayTraderSharedLib'], ['classPath', '/opt/IBM/sharedlibs/daytrader/'], ['isolatedClassLoader', 'false'],
     ['description', 'Site-wide jars for DayTrader (logging config, Db2 helpers)']])
AdminConfig.save()
