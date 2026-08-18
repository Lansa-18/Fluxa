#![no_std]

use soroban_sdk::{contract, contracterror, contractimpl, contracttype, symbol_short, token, Address, Env, String, Vec};

#[derive(Clone)]
#[contracttype]
pub enum DataKey {
    Owner,
    Guardians,
    RecoveryThreshold,
    SpendingLimit,
    SpendingWindowSeconds,
    SpentInWindow,
    WindowStart,
    TimeLockUntil,
    PendingRecovery,
    NextProposalId,
}

#[derive(Clone)]
#[contracttype]
pub struct RecoveryProposal {
    pub id: u32,
    pub new_owner: Address,
    pub signatures: Vec<Address>,
}

#[derive(Clone)]
#[contracttype]
pub struct WalletState {
    pub owner: Address,
    pub guardians: Vec<Address>,
    pub recovery_threshold: u32,
    pub spending_limit: i128,
    pub spending_window_seconds: u64,
    pub spent_in_window: i128,
    pub window_start: u64,
    pub time_lock_until: u64,
    pub has_pending_recovery: bool,
}

#[derive(Clone)]
#[contracttype]
pub struct SpendingStatus {
    pub limit: i128,
    pub spent_in_window: i128,
    pub remaining: i128,
    pub window_resets_at: u64,
}

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum WalletError {
    NotInitialized = 1,
    AlreadyInitialized = 2,
    InvalidThreshold = 3,
    NotGuardian = 4,
    SpendingLimitExceeded = 5,
    TimeLocked = 6,
    NoPendingRecovery = 7,
    ProposalMismatch = 8,
    AlreadySigned = 9,
    GuardianExists = 10,
    GuardianNotFound = 11,
}

#[contract]
pub struct ContractWallet;

#[contractimpl]
impl ContractWallet {
    /// Runs at deployment so a wallet can never exist on-chain in an
    /// uninitialized state, which would otherwise leave a window for someone
    /// else to claim ownership by calling `initialize` first.
    pub fn __constructor(
        env: Env,
        owner: Address,
        guardians: Vec<Address>,
        recovery_threshold: u32,
        spending_limit: i128,
        spending_window_seconds: u64,
    ) -> Result<(), WalletError> {
        Self::init(
            &env,
            owner,
            guardians,
            recovery_threshold,
            spending_limit,
            spending_window_seconds,
        )
    }

    /// Callable once. Sets up the owner, guardian set, recovery threshold and
    /// initial spending limit/window. The deploying owner must authorize.
    pub fn initialize(
        env: Env,
        owner: Address,
        guardians: Vec<Address>,
        recovery_threshold: u32,
        spending_limit: i128,
        spending_window_seconds: u64,
    ) -> Result<(), WalletError> {
        Self::init(
            &env,
            owner,
            guardians,
            recovery_threshold,
            spending_limit,
            spending_window_seconds,
        )
    }

    fn init(
        env: &Env,
        owner: Address,
        guardians: Vec<Address>,
        recovery_threshold: u32,
        spending_limit: i128,
        spending_window_seconds: u64,
    ) -> Result<(), WalletError> {
        if env.storage().instance().has(&DataKey::Owner) {
            return Err(WalletError::AlreadyInitialized);
        }
        if guardians.is_empty() {
            if recovery_threshold != 0 {
                return Err(WalletError::InvalidThreshold);
            }
        } else if recovery_threshold == 0 || recovery_threshold > guardians.len() {
            return Err(WalletError::InvalidThreshold);
        }

        owner.require_auth();

        let now = env.ledger().timestamp();

        env.storage().instance().set(&DataKey::Owner, &owner);
        env.storage().instance().set(&DataKey::Guardians, &guardians);
        env.storage().instance().set(&DataKey::RecoveryThreshold, &recovery_threshold);
        env.storage().instance().set(&DataKey::SpendingLimit, &spending_limit);
        env.storage().instance().set(&DataKey::SpendingWindowSeconds, &spending_window_seconds);
        env.storage().instance().set(&DataKey::SpentInWindow, &0i128);
        env.storage().instance().set(&DataKey::WindowStart, &now);
        env.storage().instance().set(&DataKey::TimeLockUntil, &0u64);
        env.storage().instance().set(&DataKey::NextProposalId, &0u32);

        env.events().publish((symbol_short!("wal_init"),), owner.clone());

        Ok(())
    }

    /// Owner-only payment. Enforces the rolling spending window/limit and any
    /// active time-lock before transferring `asset` to `destination`.
    pub fn execute_payment(
        env: Env,
        destination: Address,
        asset: Address,
        amount: i128,
        memo: Option<String>,
    ) -> Result<(), WalletError> {
        let owner = Self::require_owner(&env)?;
        owner.require_auth();

        let now = env.ledger().timestamp();
        let time_lock_until: u64 = env.storage().instance().get(&DataKey::TimeLockUntil).unwrap_or(0);
        if time_lock_until > now {
            return Err(WalletError::TimeLocked);
        }

        let spending_window_seconds: u64 =
            env.storage().instance().get(&DataKey::SpendingWindowSeconds).unwrap();
        let mut window_start: u64 = env.storage().instance().get(&DataKey::WindowStart).unwrap();
        let mut spent_in_window: i128 = env.storage().instance().get(&DataKey::SpentInWindow).unwrap();

        if now > window_start + spending_window_seconds {
            window_start = now;
            spent_in_window = 0;
        }

        let spending_limit: i128 = env.storage().instance().get(&DataKey::SpendingLimit).unwrap();
        let new_spent = spent_in_window + amount;
        if new_spent > spending_limit {
            return Err(WalletError::SpendingLimitExceeded);
        }

        let token_client = token::Client::new(&env, &asset);
        token_client.transfer(&env.current_contract_address(), &destination, &amount);

        env.storage().instance().set(&DataKey::SpentInWindow, &new_spent);
        env.storage().instance().set(&DataKey::WindowStart, &window_start);

        env.events()
            .publish((symbol_short!("pay_exec"), destination), (amount, memo));

        Ok(())
    }

    /// Any guardian may open a recovery proposal for a new owner. The
    /// proposing guardian's approval counts as the first signature.
    pub fn propose_recovery(env: Env, guardian: Address, new_owner: Address) -> Result<u32, WalletError> {
        guardian.require_auth();
        Self::require_guardian(&env, &guardian)?;

        let next_id: u32 = env.storage().instance().get(&DataKey::NextProposalId).unwrap_or(0);

        let mut signatures = Vec::new(&env);
        signatures.push_back(guardian.clone());

        let proposal = RecoveryProposal {
            id: next_id,
            new_owner: new_owner.clone(),
            signatures,
        };
        env.storage().instance().set(&DataKey::PendingRecovery, &proposal);
        env.storage().instance().set(&DataKey::NextProposalId, &(next_id + 1));

        env.events()
            .publish((symbol_short!("rec_prop"),), (next_id, new_owner));

        Ok(next_id)
    }

    /// A guardian who has not yet signed the pending proposal approves it.
    /// Once `recovery_threshold` signatures are collected, ownership
    /// transfers immediately and the guardian list is cleared.
    pub fn approve_recovery(env: Env, guardian: Address, proposal_id: u32) -> Result<(), WalletError> {
        guardian.require_auth();
        Self::require_guardian(&env, &guardian)?;

        let mut proposal: RecoveryProposal = env
            .storage()
            .instance()
            .get(&DataKey::PendingRecovery)
            .ok_or(WalletError::NoPendingRecovery)?;

        if proposal.id != proposal_id {
            return Err(WalletError::ProposalMismatch);
        }
        if proposal.signatures.contains(&guardian) {
            return Err(WalletError::AlreadySigned);
        }
        proposal.signatures.push_back(guardian.clone());

        let threshold: u32 = env.storage().instance().get(&DataKey::RecoveryThreshold).unwrap();

        if proposal.signatures.len() >= threshold {
            env.storage().instance().set(&DataKey::Owner, &proposal.new_owner);
            env.storage()
                .instance()
                .set(&DataKey::Guardians, &Vec::<Address>::new(&env));
            env.storage().instance().remove(&DataKey::PendingRecovery);
            env.events()
                .publish((symbol_short!("rec_exec"),), proposal.new_owner);
        } else {
            env.storage().instance().set(&DataKey::PendingRecovery, &proposal);
        }

        Ok(())
    }

    /// Owner-only. Does not itself check whether a lock is already active —
    /// callers use this to both set and clear (`until_timestamp = 0`) the lock.
    pub fn set_time_lock(env: Env, until_timestamp: u64) -> Result<(), WalletError> {
        let owner = Self::require_owner(&env)?;
        owner.require_auth();

        env.storage().instance().set(&DataKey::TimeLockUntil, &until_timestamp);
        env.events().publish((symbol_short!("lock_set"),), until_timestamp);

        Ok(())
    }

    /// Owner-only; rejected while a time-lock is active, same check as
    /// `execute_payment`.
    pub fn add_guardian(env: Env, guardian: Address) -> Result<(), WalletError> {
        let owner = Self::require_owner(&env)?;
        owner.require_auth();
        Self::reject_if_locked(&env)?;

        let mut guardians: Vec<Address> = env.storage().instance().get(&DataKey::Guardians).unwrap();
        if guardians.contains(&guardian) {
            return Err(WalletError::GuardianExists);
        }
        guardians.push_back(guardian.clone());
        env.storage().instance().set(&DataKey::Guardians, &guardians);

        env.events().publish((symbol_short!("g_added"),), guardian);

        Ok(())
    }

    /// Owner-only; rejected while a time-lock is active, same check as
    /// `execute_payment`.
    pub fn remove_guardian(env: Env, guardian: Address) -> Result<(), WalletError> {
        let owner = Self::require_owner(&env)?;
        owner.require_auth();
        Self::reject_if_locked(&env)?;

        let guardians: Vec<Address> = env.storage().instance().get(&DataKey::Guardians).unwrap();
        match guardians.iter().position(|g| g == guardian) {
            Some(idx) => {
                let mut updated = guardians.clone();
                updated.remove(idx as u32);
                env.storage().instance().set(&DataKey::Guardians, &updated);
                env.events().publish((symbol_short!("g_removed"),), guardian);
                Ok(())
            }
            None => Err(WalletError::GuardianNotFound),
        }
    }

    /// Full decoded contract state, for off-chain adapters to read.
    pub fn get_state(env: Env) -> Result<WalletState, WalletError> {
        let owner = Self::require_owner(&env)?;
        Ok(WalletState {
            owner,
            guardians: env.storage().instance().get(&DataKey::Guardians).unwrap(),
            recovery_threshold: env.storage().instance().get(&DataKey::RecoveryThreshold).unwrap(),
            spending_limit: env.storage().instance().get(&DataKey::SpendingLimit).unwrap(),
            spending_window_seconds: env
                .storage()
                .instance()
                .get(&DataKey::SpendingWindowSeconds)
                .unwrap(),
            spent_in_window: env.storage().instance().get(&DataKey::SpentInWindow).unwrap(),
            window_start: env.storage().instance().get(&DataKey::WindowStart).unwrap(),
            time_lock_until: env.storage().instance().get(&DataKey::TimeLockUntil).unwrap(),
            has_pending_recovery: env.storage().instance().has(&DataKey::PendingRecovery),
        })
    }

    /// The pending recovery proposal, if any.
    pub fn get_pending_recovery(env: Env) -> Option<RecoveryProposal> {
        env.storage().instance().get(&DataKey::PendingRecovery)
    }

    /// Current spending window, reflecting a window reset that would happen
    /// on the next payment without mutating storage.
    pub fn get_spending_status(env: Env) -> Result<SpendingStatus, WalletError> {
        Self::require_owner(&env)?;

        let now = env.ledger().timestamp();
        let spending_window_seconds: u64 =
            env.storage().instance().get(&DataKey::SpendingWindowSeconds).unwrap();
        let window_start: u64 = env.storage().instance().get(&DataKey::WindowStart).unwrap();
        let spent_in_window: i128 = env.storage().instance().get(&DataKey::SpentInWindow).unwrap();
        let spending_limit: i128 = env.storage().instance().get(&DataKey::SpendingLimit).unwrap();

        let (spent, resets_at) = if now > window_start + spending_window_seconds {
            (0i128, now + spending_window_seconds)
        } else {
            (spent_in_window, window_start + spending_window_seconds)
        };

        Ok(SpendingStatus {
            limit: spending_limit,
            spent_in_window: spent,
            remaining: spending_limit - spent,
            window_resets_at: resets_at,
        })
    }

    fn require_owner(env: &Env) -> Result<Address, WalletError> {
        env.storage()
            .instance()
            .get(&DataKey::Owner)
            .ok_or(WalletError::NotInitialized)
    }

    fn require_guardian(env: &Env, guardian: &Address) -> Result<(), WalletError> {
        let guardians: Vec<Address> = env.storage().instance().get(&DataKey::Guardians).unwrap();
        if guardians.contains(guardian) {
            Ok(())
        } else {
            Err(WalletError::NotGuardian)
        }
    }

    fn reject_if_locked(env: &Env) -> Result<(), WalletError> {
        let time_lock_until: u64 = env.storage().instance().get(&DataKey::TimeLockUntil).unwrap_or(0);
        if time_lock_until > env.ledger().timestamp() {
            Err(WalletError::TimeLocked)
        } else {
            Ok(())
        }
    }
}

#[cfg(test)]
mod test;
