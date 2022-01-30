# Example configuration
The example below gives roughly a compression ratio of 4:1.
```bash
# content of base folder will be replaced with a compressed zfs volume, save content for later use
sudo mv /var/lib/docker/volumes/midgard_pg/_data/base /tmp
sudo apt install zfsutils-linux
# check zfs installation
whereis zfs
# create empty 24G file, will be used as loopback device to store the filesystem
dd if=/dev/zero of=/tmp/zfs_image bs=24576 count=1048576
# create loopback device from file
sudo losetup -fP /tmp/zfs_image
# find assigned devicename
losetup -a
# create a storage pool named "pg"
sudo zpool create -o autoexpand=on pg /dev/loop12
sudo zpool set feature@lz4_compress=enabled pg
# create inside pg pool a storage volume accessible from the host via the mountpoint folder
sudo zfs create -o recordsize=32k -o redundant_metadata=most -o primarycache=metadata -o logbias=throughput pg/base -o mountpoint=/var/lib/docker/volumes/midgard_pg/_data/base
sudo zfs set compression=lz4 pg/base
# copy existing postgresql content back into the mountpoint, this will be compressed now
sudo cp -r /tmp/base /var/lib/docker/volumes/midgard_pg/_data
# check compression ratio
sudo zfs get all | grep compress
```
## Notes
The zfs block size in the example is zfsBlockSize=32K, which is above the default postgresql block size, postgreSqlBlockSize=8K.
Such zfsBlockSize gives a better compression, but creates write amplification, since zfs is a copy on write filesystem
(each update to a block creates a new block copy+update, and the unit of copy is the zfsBlockSize).
Ideally the two block sizes, zfsBlockSize and postgreSqlBlockSize should match, but the postgresql block size can only be defined when creating a new postgresql compilation.
Additionally the limit on the postgreSqlBlockSize is 32K.

