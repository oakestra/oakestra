#!/bin/bash

CURRENT=$(cat override-alpha-versions.yaml | grep alpha- -m 1 | cut -d ':' -f 3 | sed 's/ //g')
function get_latest_alpha_tag() {
  repository_url="https://api.github.com/repos/oakestra/oakestra"
  curl -s "$repository_url/tags" | grep "alpha-" -m 1 | sed 's/"//g' | cut -d ':' -f 2 | sed 's/,//g' | sed 's/ //g'
}
LATEST=$(get_latest_alpha_tag)

echo Current tag: $CURRENT
echo Latest tag: $LATEST

if [[ "$CURRENT" != "$LATEST" ]]; then

  printf "Do you want to update with the latest alpha? (y/n) "
  read answer

  if [[ $answer == "y" ]]; then
    # The user wants to continue, so do nothing
    echo "Updating..."
    cp override-alpha-versions.yaml override-alpha-versions.old.$CURRENT.yaml
    sed "s/$CURRENT/$LATEST/g" override-alpha-versions.yaml
    echo "💎 DONE 💎"
  else
    # The user does not want to continue, so kill the child process
    echo "Nothing changed"
  fi
else
  echo "Everything up to date 🥳"
fi





