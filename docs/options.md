## setup-secrets\.enable



Whether to enable the cli utility for setting up secrets\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.autoSetup

Whether to enable automatic fetching \& storing of secrets\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations



List of commands to run when the form is submitted\.



*Type:*
list of (submodule)

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.enable



Whether to enable this secret destination



*Type:*
boolean



*Default:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.cmd



Command to create or update the secret



*Type:*
string

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.logPrefix



A short description of the destination\.



*Type:*
string



*Default:*

```nix
""
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.requires



List of secrets this command requires in order to execute



*Type:*
list of string



*Default:*

```nix
[ ]
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.wants



List of secrets this command wants (but not requires)



*Type:*
list of string



*Default:*

```nix
[ ]
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.script



The generated setup-secrets script



*Type:*
package *(read only)*



*Default:*

```nix
""
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources



Map of secrets\. \<name> becomes the secret name\.



*Type:*
attribute set of (submodule)

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.enable



Whether to enable this secret source



*Type:*
boolean



*Default:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.cmd



Command to retrieve the secret



*Type:*
null or string



*Default:*

```nix
null
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.description



The description of the secret as displayed in the form\. Defaults to \<name>



*Type:*
string



*Default:*

```nix
"‹name›"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.users\.enable



Whether to enable management of user hashed password files through setup-secrets\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/users\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/users.nix)


