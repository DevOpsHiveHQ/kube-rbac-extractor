#! /bin/bash

tmp_dir=/tmp/kubernetes

# Clone K8s repo.
test -d ${tmp_dir} ||
    git clone --depth=1 https://github.com/kubernetes/kubernetes ${tmp_dir}

# Get the needed keys from the api resources.
ls ${tmp_dir}/api/discovery/*__v1* | while read api_file; do
    cat ${api_file} | jq --arg groupVersion "$(jq -r '.groupVersion' ${api_file} | cut -d'/' -f1)" '
    .resources | map({
        groupVersion: $groupVersion,
        kind,
        name,
        verbs
    })
    ' > apis/$(basename ${api_file})
done
