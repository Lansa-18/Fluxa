use super::*;
use soroban_sdk::testutils::{Address as _, Ledger};
use soroban_sdk::Env;

fn setup(
    env: &Env,
    guardians: &Vec<Address>,
    threshold: u32,
    spending_limit: i128,
    window_seconds: u64,
) -> (Address, Address) {
    let owner = Address::generate(env);
    // Deployment runs __constructor, which is the same path the Go adapter
    // uses via CreateContractV2 constructor args.
    let contract_id = env.register(
        ContractWallet,
        (
            owner.clone(),
            guardians.clone(),
            threshold,
            spending_limit,
            window_seconds,
        ),
    );
    (contract_id, owner)
}

fn issue_token(env: &Env, admin: &Address, holder: &Address, amount: i128) -> Address {
    let sac = env.register_stellar_asset_contract_v2(admin.clone());
    let asset = sac.address();
    let asset_admin_client = token::StellarAssetClient::new(env, &asset);
    asset_admin_client.mint(holder, &amount);
    asset
}

#[test]
fn initialize_is_rejected_once_the_wallet_is_already_set_up() {
    let env = Env::default();
    env.mock_all_auths();

    let guardians = Vec::new(&env);
    let (contract_id, owner) = setup(&env, &guardians, 0, 100, 3600);
    let client = ContractWalletClient::new(&env, &contract_id);

    let attacker = Address::generate(&env);
    let result = client.try_initialize(&attacker, &guardians, &0, &100, &3600);
    assert_eq!(result, Err(Ok(WalletError::AlreadyInitialized)));

    assert_eq!(client.get_state().owner, owner);
}

#[test]
fn spending_limit_rejects_when_cumulative_amount_exceeds_limit() {
    let env = Env::default();
    env.mock_all_auths();

    let guardians = Vec::new(&env);
    let (contract_id, owner) = setup(&env, &guardians, 0, 100, 3600);
    let client = ContractWalletClient::new(&env, &contract_id);

    let asset = issue_token(&env, &owner, &contract_id, 1_000);
    let destination = Address::generate(&env);

    // First payment of 60 is within the 100 limit.
    client.execute_payment(&destination, &asset, &60, &None);

    // Second payment of 60 would bring cumulative spend to 120 > 100.
    let result = client.try_execute_payment(&destination, &asset, &60, &None);
    assert_eq!(result, Err(Ok(WalletError::SpendingLimitExceeded)));
}

#[test]
fn spending_window_resets_after_it_elapses() {
    let env = Env::default();
    env.mock_all_auths();

    let guardians = Vec::new(&env);
    let (contract_id, owner) = setup(&env, &guardians, 0, 100, 3600);
    let client = ContractWalletClient::new(&env, &contract_id);

    let asset = issue_token(&env, &owner, &contract_id, 1_000);
    let destination = Address::generate(&env);

    client.execute_payment(&destination, &asset, &90, &None);

    // Still inside the window: a further 90 would exceed the limit.
    let blocked = client.try_execute_payment(&destination, &asset, &90, &None);
    assert_eq!(blocked, Err(Ok(WalletError::SpendingLimitExceeded)));

    // Advance the ledger clock past the spending window.
    let now = env.ledger().timestamp();
    env.ledger().set_timestamp(now + 3601);

    // The window has reset, so a fresh 90 payment succeeds.
    client.execute_payment(&destination, &asset, &90, &None);

    let status = client.get_spending_status();
    assert_eq!(status.spent_in_window, 90);
}

#[test]
fn recovery_requires_exactly_threshold_approvals() {
    let env = Env::default();
    env.mock_all_auths();

    let guardian_a = Address::generate(&env);
    let guardian_b = Address::generate(&env);
    let guardian_c = Address::generate(&env);
    let mut guardians = Vec::new(&env);
    guardians.push_back(guardian_a.clone());
    guardians.push_back(guardian_b.clone());
    guardians.push_back(guardian_c.clone());

    let (contract_id, owner) = setup(&env, &guardians, 2, 100, 3600);
    let client = ContractWalletClient::new(&env, &contract_id);

    let new_owner = Address::generate(&env);
    let proposal_id = client.propose_recovery(&guardian_a, &new_owner);

    // One approval (the proposer's own) is not enough to change the owner.
    let state = client.get_state();
    assert_eq!(state.owner, owner);

    client.approve_recovery(&guardian_b, &proposal_id);

    let state = client.get_state();
    assert_eq!(state.owner, new_owner);
    assert_eq!(state.guardians.len(), 0);
}

#[test]
fn time_lock_blocks_payment_and_add_guardian_but_not_recovery_proposal() {
    let env = Env::default();
    env.mock_all_auths();

    let guardian_a = Address::generate(&env);
    let mut guardians = Vec::new(&env);
    guardians.push_back(guardian_a.clone());

    let (contract_id, owner) = setup(&env, &guardians, 1, 100, 3600);
    let client = ContractWalletClient::new(&env, &contract_id);

    let asset = issue_token(&env, &owner, &contract_id, 1_000);
    let destination = Address::generate(&env);

    let now = env.ledger().timestamp();
    client.set_time_lock(&(now + 1_000));

    let payment_result = client.try_execute_payment(&destination, &asset, &10, &None);
    assert_eq!(payment_result, Err(Ok(WalletError::TimeLocked)));

    let guardian_b = Address::generate(&env);
    let add_guardian_result = client.try_add_guardian(&guardian_b);
    assert_eq!(add_guardian_result, Err(Ok(WalletError::TimeLocked)));

    // propose_recovery must still succeed while locked.
    let new_owner = Address::generate(&env);
    let proposal_id = client.propose_recovery(&guardian_a, &new_owner);
    assert_eq!(proposal_id, 0);
}
