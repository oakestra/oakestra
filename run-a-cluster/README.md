## Customize run-a-cluster with alpha images

It's possible to use the compose override functionality to customize the deployment and use the alpha images instead of the main ones. 

`docker-compose -f 1-DOC.yml -f override-alpha-versions.yaml up`

The override contains pre configured versions of the alpha images, you can manually update it or you can use `./update-alpha-override-with-latest-alpha.sh` to automatically use the latest alpha images available. 
