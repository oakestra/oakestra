import ext_requests.mongodb_client as db
from bson import ObjectId


def mongo_add_organization(organization):
    db.app.logger.info("MONGODB - insert organization...")
    new_orga = db.mongo_organization.insert_one(organization)
    inserted_id = new_orga.inserted_id
    db.app.logger.info("MONGODB - organization {} inserted".format(str(inserted_id)))
    return str(inserted_id)


def mongo_get_all_organizations():
    return list(db.mongo_organization.find())


def mongo_update_organizations(organization_id, organization):
    db.app.logger.info("MONGODB - update organization...")
    organization = db.mongo_organization.find_one_and_update(
        {"_id": ObjectId(organization_id)},
        {
            "$set": {
                "name": organization.get("name"),
                "member": organization.get("member"),
            }
        },
        return_document=True,
    )
    db.app.logger.info("MONGODB - organization updated")
    return organization


def mongo_delete_organization(organization_id):
    db.app.logger.info("MONGODB - delete organization...")
    db.mongo_organization.find_one_and_delete({"_id": ObjectId(organization_id)})
    db.app.logger.info("MONGODB - organization deleted")
    return db.mongo_organization.find()


def mongo_get_organization_by_name(organization_name):
    organization = db.mongo_organization.find_one({"name": organization_name})
    print(organization)
    if organization is None:
        return None
    return organization


def mongo_get_roles_of_user_in_organization(user_id, organization_id):
    organization = db.mongo_organization.find_one({"_id": ObjectId(organization_id)})
    roles = []
    for m in organization["member"]:
        if m["user_id"] == user_id:
            roles = m["roles"]
    return roles


def mongo_delete_all_role_entrys_of_user(user_id):
    organization = list(db.mongo_organization.find())
    for o in organization:
        member = o["member"]
        filtered = list(filter(lambda m: m["user_id"] != user_id, member))
        db.mongo_organization.find_one_and_update(
            {"_id": ObjectId(o["_id"])},
            {"$set": {"member": filtered}},
            return_document=True,
        )


def mongo_delete_role_entry(user_id, organization_id):
    organization = db.mongo_organization.find_one({"_id": ObjectId(organization_id)})
    member = organization["member"]
    filtered = list(filter(lambda m: m["user_id"] != user_id, member))
    return db.mongo_organization.find_one_and_update(
        {"_id": ObjectId(organization["_id"])},
        {"$set": {"member": filtered}},
        return_document=True,
    )


def mongo_add_user_role_to_organization(user_id, organization_id, roles):
    organization = db.mongo_organization.find_one({"_id": ObjectId(organization_id)})
    member = list(filter(lambda m: m["user_id"] != user_id, organization["member"]))
    member.append({"user_id": user_id, "roles": roles})
    return db.mongo_organization.find_one_and_update(
        {"_id": ObjectId(organization_id)},
        {"$set": {"member": member}},
        return_document=True,
    )
