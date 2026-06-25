import * as core from '@actions/core';
import axios from 'axios';
import https from 'https';
import sslRootCAs from 'ssl-root-cas';

https.globalAgent.options.ca = sslRootCAs.create();

const POLL_INTERVAL_MS = 60_000;
const TIMEOUT_MS = 2 * 60 * 60 * 1000; // 2 hours

async function triggerAWX() {
  try {
    const awxUrl = core.getInput('AWX_URL');
    const token = core.getInput('AWX_TOKEN');
    const workflowTemplateId = core.getInput('AWX_TEMPLATE_ID');
    const pullRequestBranch = core.getInput('PR_BRANCH');
    const pullRequestCommit = core.getInput('PR_COMMIT');
    const pullRequestUser = core.getInput('PR_USER');

    const headers = {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    };

    const extraVars = {
      oak_repo_branch: pullRequestBranch,
      oak_repo_commit: pullRequestCommit,
      oak_branch: pullRequestBranch, //compatibility with oak custom workflow
      oak_commit: pullRequestCommit, //compatibility with oak custom workflow
      pr_fork_user: pullRequestUser
    };

    core.info(`🌱 Branch: ${pullRequestBranch}`);
    core.info(`#️⃣ Commit: ${pullRequestCommit}`);
    core.info(`👷 PR Commit Author: ${pullRequestUser}`);

    // Step 1: Trigger the workflow job template
    const jobLaunchUrl = `https://${awxUrl}/api/v2/workflow_job_templates/${workflowTemplateId}/launch/`;
    const response = await axios.post(jobLaunchUrl, { extra_vars: extraVars }, { headers });

    const jobId = response.data.workflow_job;
    core.info(`🆔 Execution ID: ${jobId}`);

    // Step 2: Poll the job status until terminal state or timeout
    const jobStatusUrl = `https://${awxUrl}/api/v2/workflow_jobs/${jobId}/`;
    const deadline = Date.now() + TIMEOUT_MS;

    while (Date.now() < deadline) {
      const jobResponse = await axios.get(jobStatusUrl, { headers });
      const status = jobResponse.data.status;

      core.info(`⚙️ Current job status: ${status} ⏳`);

      if (status === 'successful') {
        core.info('🎉 ✅ 🎉  Tests passed successfully! 🎉 ✅ 🎉');
        return;
      } else if (['failed', 'error', 'canceled'].includes(status)) {
        throw new Error(` 🔴 Tests execution failed with status: ${status} 🔴 `);
      }

      await new Promise(resolve => setTimeout(resolve, POLL_INTERVAL_MS));
    }

    throw new Error('Timed out waiting for AWX job to reach a terminal state');

  } catch (error) {
    core.setFailed(`Action failed: ${error.message}`);
  }
}

triggerAWX();
