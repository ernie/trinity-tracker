cd ../trinity-engine && make clean-web && cd - && BUILD_ENGINE=1 make && sudo -u quake cp -r web/dist/* /var/lib/trinity/web/
