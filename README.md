# HLS Client

## Usage

- options
    - `-addr` string : glb server addresss. (ex) 127.0.0.1:18085
    - `-count` int : the number of session. default is generation info file count
    - `-delay` int : request chunk delay(millisec)
    - `-filename` string : generation info file name
        - generation file context
        ```
        020.m3u8 172.16.32.105  OTM live    H
        020.m3u8 172.16.32.105  OTM live    H
        020.m3u8 172.16.32.105  OTM live    H
        ```

    - `-interval` int : session generation interval (second) (default 1)
    - `-playtime` int : play time (second) (default 900)
    - `-type` string : content type(live,vod) (default "live")

- example
```bash
~$ ./hls-client -addr 172.16.32.112:18080 -count 1 -filename ott_session.txt -type live
```

