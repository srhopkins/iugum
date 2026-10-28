// Only the dedicated generated UI directory is replaced. No user data is here.
import {cpSync,readdirSync,rmSync,mkdirSync} from 'node:fs'
import path from 'node:path'
const source=process.argv[2]
if(!source||!path.isAbsolute(source))throw new Error('absolute generated dist directory required')
const target=path.join(import.meta.dirname,'assets')
mkdirSync(target,{recursive:true})
for(const name of readdirSync(target))if(name!=='PLACEHOLDER')rmSync(path.join(target,name),{recursive:true,force:true})
cpSync(source,target,{recursive:true})
