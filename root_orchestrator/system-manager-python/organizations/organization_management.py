from ext_requests.organization_db import (
    mongo_add_organization,
    mongo_delete_organization,
    mongo_get_all_organizations,
    mongo_update_organizations,
)


def add_organization(organization):
    return mongo_add_organization(organization)


def update_organization(organization_id, organization):
    return mongo_update_organizations(organization_id, organization)


def delete_organization(organization_id):
    return mongo_delete_organization(organization_id)


def get_all_organizations():
    return mongo_get_all_organizations()
