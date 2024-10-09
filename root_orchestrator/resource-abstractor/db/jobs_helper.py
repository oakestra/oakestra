def build_filter(query):
    filter = query
    instance_number = filter.get("instance_number")
    if instance_number:
        filter["instance_list"] = {"$elemMatch": {"instance_number": instance_number}}
    return filter
