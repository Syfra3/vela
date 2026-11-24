# Generation retention

Publication retains sealed generations and incomplete staging directories; it does not prune history. Disk use therefore grows with each build, including SQLite and JSON copies. Readers can pin older generations, and explicit recovery considers validated sealed generations; deleting history can break those uses. Plan capacity accordingly. There is currently no automatic reclamation or supported pruning command.

Normal publication reads historical seals to identify the highest candidate, then validates candidates from highest sequence downward until one is valid. Recovery still fully validates history. Costs therefore include a directory/seal scan per publication and full artifact validation for the selected candidate; corrupt candidates can increase that cost.
