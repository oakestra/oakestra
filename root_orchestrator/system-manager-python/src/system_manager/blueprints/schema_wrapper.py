class SchemaWrapper(dict):
    def __init__(self, stuff):
        super().__init__(stuff)
        self.stuff = stuff

    def dump(self, extra_stuff):
        return extra_stuff
