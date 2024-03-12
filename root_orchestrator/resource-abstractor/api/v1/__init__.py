from api.v1 import apps_blueprint, jobs_blueprint, resources_blueprint

blueprints = [
    resources_blueprint.resourcesblp,
    apps_blueprint.applicationsblp,
    jobs_blueprint.jobsblp,
]
