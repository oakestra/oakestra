from git import Repo
import os
import stat
import subprocess


def run_unikernel_mirageos(git_url, repo_dir, filename, content):
    pull_git_repository(git_url, repo_dir)
    create_file(filename, content)
    run_file(filename)


def pull_git_repository(git_url, repo_dir):
    Repo.clone_from(git_url, repo_dir)


def create_file(filename, content):
    file = open('filename', 'w+')
    file.write(content)
    file.close()
    return file


def run_file(file):
    # make file executable
    st = os.stat(file)
    os.chmod(file, st.st_mode | stat.S_IEXEC)

    # run file
    popen = subprocess.Popen(args, stdout=subprocess.PIPE)
    popen.wait()
    output = popen.stdout.read()
    print
    output