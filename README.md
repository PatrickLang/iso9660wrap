# iso9660wrap

This turns the [iso9660wrap](https://github.com/johto/iso9660wrap) utility into a package. It provides a simple means to create an ISO9660 file containing a single file. 



## Multifile 

I'm coding this up hoping to use it with Packer & Hyper-V generation 2 VMs to create simple ISOs with small files needed to bootstrap VM setup. This could be kickstart or unattended installation files, SSH keys & scripts. It's not intended for creating bootable images or supporting other filesystems like UDF. Other tools like cdrtools are better suited for those tasks.

Build steps on Windows

```powershell
go build -o iso9660wrap.exe .\cmd\main.go
```

Example run

```powershell
.\iso9660wrap.exe -o temp.iso .\iso9660_writer.go .\iso9660wrap.go
2018/01/01 21:55:04 error: WriteFiles is still a work in progress
```


### Current work

- [x] Bug: Something is amiss in the file/directory records. Files are right length but all 0's in my implementation
- [x] Fix old syntax
- [ ] Filename/directory entry sorting to spec
- [ ] Eventually - multiple directories


## References

http://www.idea2ic.com/File_Formats/iso9660.pdf